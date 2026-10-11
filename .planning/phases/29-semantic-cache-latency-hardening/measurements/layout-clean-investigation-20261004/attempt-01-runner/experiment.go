package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/crypto/ssh"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const fixturePrefix = "veloxmesh-layout-20261004-01a103bf-"
const trialIdle = 60 * time.Second
const taskLabel = "01a103bf-layout-clean"

type runner struct {
	Client                        *ssh.Client
	Key, HelperImage, QdrantImage string
	Created                       []string
	Events                        *os.File
}

func runExperiment() (result error) {
	client, err := connectInventory()
	if err != nil {
		return err
	}
	defer client.Close()
	timer := time.AfterFunc(30*time.Minute, func() { client.Close() })
	defer timer.Stop()
	events, err := os.Create(filepath.Join(inventoryRoot, "events.jsonl"))
	if err != nil {
		return err
	}
	defer events.Close()
	r := &runner{Client: client, Events: events}
	stopHost, err := startHostMonitor()
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, stopHost()) }()
	defer func() { result = errors.Join(result, r.cleanup()) }()
	if err := r.preflight(); err != nil {
		return err
	}
	stopVM, err := r.startObserver("continuous", 0)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, stopVM()) }()
	for _, arm := range []string{"shared", "many"} {
		if err := r.prepare(arm); err != nil {
			return err
		}
	}
	for index, arm := range []string{"shared", "many", "many", "shared", "shared", "many"} {
		name := fmt.Sprintf("trial-%02d-%s", index+1, arm)
		if err := r.trial(name, arm); err != nil {
			return err
		}
	}
	return r.event("matrix-complete", map[string]any{"trials": 6, "release_approval": false})
}

func (r *runner) event(kind string, detail any) error {
	return json.NewEncoder(r.Events).Encode(map[string]any{"type": kind, "utc": time.Now().UTC(), "detail": detail})
}

func (r *runner) preflight() error {
	active, err := smallCommand(r.Client, "docker ps -q")
	if err != nil {
		return err
	}
	if strings.TrimSpace(active) != "" {
		return fmt.Errorf("running-container collision")
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	r.Key = hex.EncodeToString(key)
	for name, target := range map[string]*string{"veloxmesh-test-postgres": &r.HelperImage, "veloxmesh-test-qdrant": &r.QdrantImage} {
		value, err := smallCommand(r.Client, "docker inspect --format '{{.Image}}' "+name)
		if err != nil {
			return err
		}
		*target = strings.TrimSpace(value)
	}
	file, err := os.Open(".tmp/layout-clean-investigation-20261004/probe-linux-v2")
	if err != nil {
		return err
	}
	defer file.Close()
	session, err := r.Client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	session.Stdin = file
	if err := session.Run("test ! -e " + remoteLayoutDirectory + "/probe-v2 && umask 077 && cat > " + remoteLayoutDirectory + "/probe-v2 && chmod 700 " + remoteLayoutDirectory + "/probe-v2"); err != nil {
		return err
	}
	return r.event("preflight", map[string]any{"image": r.QdrantImage, "fixture_seed": 29, "points": 16542, "dimension": 768, "orders": []string{"A/B", "B/A", "A/B"}})
}

func (r *runner) engine(path string, body any) ([]byte, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	session, err := r.Client.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()
	session.Stdin = strings.NewReader(string(data))
	return session.CombinedOutput("curl --silent --show-error --fail-with-body --max-time 120 --unix-socket /var/run/docker.sock -H 'Content-Type: application/json' -X POST --data-binary @- 'http://localhost" + path + "'")
}

func (r *runner) create(arm string) error {
	name := fixturePrefix + arm
	existing, err := smallCommand(r.Client, "docker volume ls --format '{{.Name}}'")
	if err != nil {
		return err
	}
	for _, line := range strings.Fields(existing) {
		if line == name {
			return fmt.Errorf("fixture volume already exists")
		}
	}
	if _, err := r.engine("/volumes/create", map[string]any{"Name": name, "Labels": map[string]string{"veloxmesh.owner": taskLabel}}); err != nil {
		return err
	}
	body := map[string]any{"Image": r.QdrantImage, "Env": []string{"QDRANT__SERVICE__API_KEY=" + r.Key}, "Labels": map[string]string{"veloxmesh.owner": taskLabel}, "ExposedPorts": map[string]any{"6333/tcp": map[string]any{}}, "HostConfig": map[string]any{"Binds": []string{name + ":/qdrant/storage", remoteLayoutDirectory + "/probe-v2:/probe:ro"}, "PortBindings": map[string]any{"6333/tcp": []any{map[string]string{"HostIp": "127.0.0.1", "HostPort": "16333"}}}}}
	if _, err := r.engine("/containers/create?name="+name, body); err != nil {
		return err
	}
	r.Created = append(r.Created, name)
	return captureInventory(r.Client, "empty-"+arm+".log", "docker run --rm --pull never --read-only --network none --mount type=volume,source="+name+",target=/fixture,readonly --entrypoint /bin/sh "+r.HelperImage+" -c 'test -z \"$(ls -A /fixture)\" && echo fixture-empty'")
}

func (r *runner) config(arm, action string) map[string]any {
	count := 1
	if arm == "many" {
		count = 323
	}
	return map[string]any{"action": action, "base_url": "http://127.0.0.1:16333", "api_key": r.Key, "collections": count, "points": 16542, "dimension": 768, "seed": 29, "queries": 100}
}

func (r *runner) prepare(arm string) error {
	if err := r.create(arm); err != nil {
		return err
	}
	name := fixturePrefix + arm
	if _, err := smallCommand(r.Client, "docker start "+name); err != nil {
		return err
	}
	if err := invokeProbe(r.Client, invocation{"prepare-ready-" + arm + ".jsonl", remoteLayoutDirectory + "/probe-v2", r.config(arm, "ready")}); err != nil {
		return err
	}
	config := r.config(arm, "prepare")
	config["base_url"] = "http://127.0.0.1:6333"
	if err := invokeProbe(r.Client, invocation{"prepare-" + arm + ".jsonl", "docker exec -i " + name + " /probe", config}); err != nil {
		return err
	}
	if err := r.verifyDigests(arm); err != nil {
		return err
	}
	if _, err := smallCommand(r.Client, "docker stop -t 30 "+name); err != nil {
		return err
	}
	return r.event("fixture-stopped", map[string]any{"arm": arm})
}

func (r *runner) verifyDigests(arm string) error {
	data, err := os.ReadFile(filepath.Join(inventoryRoot, "prepare-"+arm+".jsonl"))
	if err != nil {
		return err
	}
	var last map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if err := json.Unmarshal([]byte(line), &last); err != nil {
			return err
		}
	}
	if last["type"] != "fixture-complete" {
		return fmt.Errorf("missing fixture completion")
	}
	digest := last["request_digest"]
	if arm == "many" {
		shared, err := os.ReadFile(filepath.Join(inventoryRoot, "digest-shared.json"))
		if err != nil {
			return err
		}
		var saved any
		if err := json.Unmarshal(shared, &saved); err != nil {
			return err
		}
		if saved != digest {
			return fmt.Errorf("fixture digests differ")
		}
	}
	encoded, err := json.Marshal(digest)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(inventoryRoot, "digest-"+arm+".json"), encoded, 0600)
}

func (r *runner) trialBaseline(name string) error {
	for _, fixture := range []string{"shared", "many"} {
		if err := invokeProbe(r.Client, invocation{name + "-evict-" + fixture + ".jsonl", evictionCommand(r.HelperImage, fixturePrefix+fixture), map[string]any{"action": "evict"}}); err != nil {
			return err
		}
	}
	if err := invokeProbe(r.Client, invocation{name + "-gate.jsonl", remoteLayoutDirectory + "/probe-v2", map[string]any{"action": "gate"}}); err != nil {
		return err
	}
	return nil
}

func (r *runner) trial(name, arm string) (result error) {
	if err := r.trialBaseline(name); err != nil {
		return err
	}
	if err := r.event("trial-start", map[string]any{"trial": name, "arm": arm}); err != nil {
		return err
	}
	start := time.Now()
	if _, err := smallCommand(r.Client, "docker start "+fixturePrefix+arm); err != nil {
		return err
	}
	stop, err := r.containerObserver(name, arm)
	if err != nil {
		return err
	}
	observerStopped := false
	defer func() {
		if !observerStopped {
			result = errors.Join(result, stop())
		}
	}()
	if err := invokeProbe(r.Client, invocation{name + "-ready.jsonl", remoteLayoutDirectory + "/probe-v2", r.config(arm, "ready")}); err != nil {
		return err
	}
	if err := r.event("trial-ready", map[string]any{"trial": name, "start_to_ready_ms": float64(time.Since(start).Microseconds()) / 1000}); err != nil {
		return err
	}
	fmt.Println(name, "ready; observing fixed 60s idle")
	time.Sleep(trialIdle)
	if err := r.event("query-start", map[string]any{"trial": name}); err != nil {
		return err
	}
	if err := invokeProbe(r.Client, invocation{name + "-queries.jsonl", remoteLayoutDirectory + "/probe-v2", r.config(arm, "query")}); err != nil {
		return err
	}
	if err := r.event("query-end", map[string]any{"trial": name}); err != nil {
		return err
	}
	observerStopped = true
	if err := stop(); err != nil {
		return err
	}
	if err := captureInventory(r.Client, name+"-logs.log", "docker logs --since '"+start.UTC().Format(time.RFC3339Nano)+"' "+fixturePrefix+arm); err != nil {
		return err
	}
	if _, err := smallCommand(r.Client, "docker stop -t 30 "+fixturePrefix+arm); err != nil {
		return err
	}
	return r.event("trial-complete", map[string]any{"trial": name})
}

func (r *runner) containerObserver(name, arm string) (func() error, error) {
	pidText, err := smallCommand(r.Client, "docker inspect --format '{{.State.Pid}}' "+fixturePrefix+arm)
	if err != nil {
		return nil, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(pidText))
	if err != nil {
		return nil, err
	}
	return r.startObserver(name, pid)
}

func (r *runner) startObserver(name string, pid int) (func() error, error) {
	path := remoteLayoutDirectory + "/stop-" + name
	session, err := r.Client.NewSession()
	if err != nil {
		return nil, err
	}
	file, err := os.Create(filepath.Join(inventoryRoot, name+"-resources.jsonl"))
	if err != nil {
		session.Close()
		return nil, err
	}
	input, _ := json.Marshal(map[string]any{"action": "observe", "pid": pid, "seconds": 1800, "stop_file": path})
	session.Stdin = strings.NewReader(string(input))
	session.Stdout = file
	session.Stderr = file
	if err := session.Start(remoteLayoutDirectory + "/probe-v2"); err != nil {
		file.Close()
		session.Close()
		return nil, err
	}
	return func() error {
		_, touchErr := smallCommand(r.Client, "touch "+path)
		waitErr := session.Wait()
		return errors.Join(touchErr, waitErr, file.Close(), session.Close())
	}, nil
}

func (r *runner) cleanup() error {
	var errorsFound []error
	for _, name := range r.Created {
		owner, err := smallCommand(r.Client, "docker inspect --format '{{index .Config.Labels \"veloxmesh.owner\"}}' "+name)
		if err != nil || strings.TrimSpace(owner) != taskLabel {
			errorsFound = append(errorsFound, fmt.Errorf("cleanup ownership verification failed: %s", name))
			continue
		}
		if _, err := smallCommand(r.Client, "docker stop -t 30 "+name); err != nil {
			errorsFound = append(errorsFound, err)
		}
	}
	errorsFound = append(errorsFound, captureInventory(r.Client, "containers-after.jsonl", "docker ps -a --format '{{json .Names}} {{json .State}}'"))
	return errors.Join(errorsFound...)
}
