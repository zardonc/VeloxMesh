package bff

import (
	"crypto/tls"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

type recordingMailSender struct {
	to      string
	subject string
	body    string
	err     error
}

func (sender *recordingMailSender) Send(to string, subject string, body string) error {
	sender.to, sender.subject, sender.body = to, subject, body
	return sender.err
}

func TestProductionVerificationUsesMailSenderWithoutReturningCode(t *testing.T) {
	sender := &recordingMailSender{}
	handler := NewServer(Config{MailSender: sender})

	registered := authRequest(t, handler, http.MethodPost, "/bff/auth/customer/register", `{
		"email":"mail@example.test",
		"username":"mail_customer",
		"organization":"Mail Security",
		"password":"correct-horse-battery-staple",
		"confirmPassword":"correct-horse-battery-staple"
	}`, nil)
	if registered.Code != http.StatusCreated {
		t.Fatalf("production registration with mail sender = %d %s", registered.Code, registered.Body.String())
	}
	if strings.Contains(strings.ToLower(registered.Body.String()), "devcode") {
		t.Fatalf("production response exposed devCode: %s", registered.Body.String())
	}
	code := regexp.MustCompile(`\b\d{6}\b`).FindString(sender.body)
	if sender.to != "mail@example.test" || code == "" {
		t.Fatalf("mail sender did not receive verification delivery: to=%q body=%q", sender.to, sender.body)
	}
	if strings.Contains(registered.Body.String(), code) {
		t.Fatalf("production response leaked delivered code: %s", registered.Body.String())
	}
	login := authRequest(t, handler, http.MethodPost, "/bff/auth/login", `{"identifier":"mail@example.test","password":"correct-horse-battery-staple"}`, nil)
	if login.Code != http.StatusOK || strings.Contains(strings.ToLower(login.Body.String()), "devcode") || regexp.MustCompile(`\b\d{6}\b`).MatchString(login.Body.String()) {
		t.Fatalf("production login leaked verification delivery data: %d %s", login.Code, login.Body.String())
	}
}

func TestProductionMailFailureReturnsServiceUnavailable(t *testing.T) {
	handler := NewServer(Config{MailSender: &recordingMailSender{err: errors.New("delivery failed")}})
	response := authRequest(t, handler, http.MethodPost, "/bff/auth/customer/register", `{
		"email":"failure@example.test",
		"username":"failure_customer",
		"organization":"Mail Failure",
		"password":"correct-horse-battery-staple",
		"confirmPassword":"correct-horse-battery-staple"
	}`, nil)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "verification_delivery_unavailable") {
		t.Fatalf("mail failure = %d %s, want safe 503", response.Code, response.Body.String())
	}
}

func TestSMTPTransportRequiresVerifiedTLS12(t *testing.T) {
	sender, err := newSMTPMailSender(SMTPConfig{
		Host: "smtp.example.test", Port: "587", Username: "mailer", Password: "secret", From: "noreply@example.test", TLSMode: "starttls",
	})
	if err != nil {
		t.Fatalf("create SMTP sender: %v", err)
	}
	config := sender.tlsConfig()
	if config.MinVersion != tls.VersionTLS12 || config.InsecureSkipVerify || config.ServerName != "smtp.example.test" {
		t.Fatalf("SMTP TLS config is not production-safe: %+v", config)
	}
	if _, err := newSMTPMailSender(SMTPConfig{Host: "smtp.example.test", Port: "25", Username: "mailer", Password: "secret", From: "noreply@example.test", TLSMode: "plain"}); err == nil {
		t.Fatal("unsafe SMTP TLS mode was accepted")
	}
}
