import requests
import json
import base64
import os
import time

URL = "http://localhost:8080/v1/chat/completions"

SANS_MODELS = [
    "mmf/mimo-auto",
    "oc/deepseek-v4-flash-free",
    "oc/mimo-v2.5-free",
    "oc/big-pickle",
    "nvidia/minimaxai/minimax-m3",
    "nvidia/deepseek-ai/deepseek-v4-pro",
    "openrouter/nvidia/nemotron-3-super-120b-a12b:free",
    "openrouter/openai/gpt-oss-120b:free",
    "openrouter/cohere/north-mini-code:free",
    "openrouter/google/gemma-4-31b-it:free"
]

GEM_MODELS = ["gemini-3.1-flash-lite"]

ALL_MODELS = [(m, "sans-primary") for m in SANS_MODELS] + [(m, "gem-primary") for m in GEM_MODELS]

# 1. Tool Calling Test
def test_tool_calling():
    print("--- 1. Tool Calling Testing ---")
    results = {}
    payload = {
        "messages": [{"role": "user", "content": "What is the weather in Tokyo?"}],
        "tools": [{
            "type": "function",
            "function": {
                "name": "get_weather",
                "description": "Get current weather",
                "parameters": {
                    "type": "object",
                    "properties": {"location": {"type": "string"}},
                    "required": ["location"]
                }
            }
        }]
    }

    for model, provider in ALL_MODELS:
        payload["model"] = model
        headers = {"Authorization": "Bearer vx-dev", "X-Provider": provider}
        try:
            resp = requests.post(URL, json=payload, headers=headers, timeout=30)
            if resp.status_code == 200:
                data = resp.json()
                msg = data.get("choices", [{}])[0].get("message", {})
                if "tool_calls" in msg or msg.get("function_call"):
                    results[model] = "PASS (Supports Tool Calling)"
                else:
                    results[model] = f"PASS (No Tool Calling, returned text: {msg.get('content')[:30]}...)"
            else:
                results[model] = f"FAIL (HTTP {resp.status_code}): {resp.text}"
        except Exception as e:
            results[model] = f"ERROR: {str(e)}"
        
        print(f"[{model}] -> {results[model]}")
    
    return results

# 2. Multimodal Test
def test_multimodal():
    print("\n--- 2. Multimodal Testing ---")
    image_path = ".assets/pic-verify.png"
    results = {}
    if not os.path.exists(image_path):
        print(f"Image {image_path} not found!")
        return results

    with open(image_path, "rb") as image_file:
        encoded_string = base64.b64encode(image_file.read()).decode('utf-8')
    
    payload = {
        "messages": [{
            "role": "user",
            "content": [
                {"type": "text", "text": "What text is in this image?"},
                {"type": "image_url", "image_url": {"url": f"data:image/png;base64,{encoded_string}"}}
            ]
        }]
    }

    # Test some models that we expect to have vision
    test_models = [("oc/deepseek-v4-flash-free", "sans-primary"), ("gemini-3.1-flash-lite", "gem-primary")]
    
    for model, provider in test_models:
        payload["model"] = model
        headers = {"Authorization": "Bearer vx-dev", "X-Provider": provider}
        try:
            resp = requests.post(URL, json=payload, headers=headers, timeout=60)
            if resp.status_code == 200:
                data = resp.json()
                content = data.get("choices", [{}])[0].get("message", {}).get("content", "")
                results[model] = f"PASS: {content}"
            else:
                results[model] = f"FAIL (HTTP {resp.status_code}): {resp.text}"
        except Exception as e:
            results[model] = f"ERROR: {str(e)}"
        
        print(f"[{model}] -> {results[model]}")

    return results

# 3. Gemini Resource Limit Handling
def test_gemini_limits():
    print("\n--- 3. Gemini Resource Limits ---")
    print("Sending rapid requests to trigger 15 RPM limit...")
    model = "gemini-3.1-flash-lite"
    provider = "gem-primary"
    payload = {"model": model, "messages": [{"role": "user", "content": "Hello"}]}
    headers = {"Authorization": "Bearer vx-dev", "X-Provider": provider}
    
    for i in range(20):
        resp = requests.post(URL, json=payload, headers=headers)
        if resp.status_code != 200:
            print(f"Request {i+1}: FAIL (HTTP {resp.status_code}) -> {resp.text}")
            return
        else:
            print(f"Request {i+1}: SUCCESS")
            # Wait slightly to not immediately block TCP connections, but fast enough to hit rate limit
            time.sleep(0.5)
    print("Did not hit rate limit within 20 requests.")

if __name__ == "__main__":
    test_tool_calling()
    test_multimodal()
    test_gemini_limits()
