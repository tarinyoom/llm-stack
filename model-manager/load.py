import json, os, sys, time, urllib.request

def env_req(name):
    v = os.environ.get(name)
    if not v:
        print(f"[fatal] missing env: {name}", file=sys.stderr)
        sys.exit(2)
    return v

def parse_duration(text):
    s = text.strip().lower()
    try:
        return float(s)
    except:
        units = {"s":1,"m":60,"h":3600}
        if len(s) >= 2 and s[-1] in units:
            try:
                return float(s[:-1]) * units[s[-1]]
            except:
                pass
    print(f"[fatal] invalid duration: {text}", file=sys.stderr)
    sys.exit(2)

def http_get_json(url, timeout):
    with urllib.request.urlopen(url, timeout=timeout) as r:
        if r.status//100!=2: raise RuntimeError(f"GET {url} {r.status}")
        return json.loads(r.read().decode())

def http_post_json(url, payload, timeout):
    data=json.dumps(payload).encode()
    req=urllib.request.Request(url,data=data,headers={"Content-Type":"application/json"})
    with urllib.request.urlopen(req,timeout=timeout) as r:
        if r.status//100!=2: raise RuntimeError(f"POST {url} {r.status}")
        txt=r.read().decode()
        return json.loads(txt) if txt else {}

def wait_for_api(base_url, startup_timeout, request_timeout):
    deadline=time.time()+startup_timeout; delay=0.5
    while True:
        try:
            http_get_json(f"{base_url}/api/tags",request_timeout); return
        except:
            if time.time()>=deadline: raise
            time.sleep(delay); delay=min(delay*1.5,5.0)

def list_tags(base_url,timeout): return http_get_json(f"{base_url}/api/tags",timeout)

def have_model(tags,name): return any(e.get("model")==name for e in tags.get("models",[]))

def ensure_model(base_url,name,timeout):
    http_post_json(f"{base_url}/api/pull",{"model":name,"stream":False},timeout)
    if not have_model(list_tags(base_url,timeout),name): raise RuntimeError(f"pull failed {name}")

def reconcile_once(base_url,models,timeout):
    if not models: print("[fatal] REQUIRED_MODELS is empty",file=sys.stderr); return -1
    tags=list_tags(base_url,timeout); changed=0
    for m in models:
        if have_model(tags,m): print(f"[ok] {m}"); continue
        print(f"[pull] {m}"); ensure_model(base_url,m,timeout); print(f"[done] {m}"); changed+=1
        tags=list_tags(base_url,timeout)
    return changed

def main():
    base = env_req("OLLAMA_BASE_URL")
    required_raw = env_req("REQUIRED_MODELS")
    startup = parse_duration(env_req("STARTUP_TIMEOUT"))
    req_timeout = parse_duration(env_req("REQUEST_TIMEOUT"))
    loop = parse_duration(env_req("LOOP_INTERVAL"))
    required = [m for m in required_raw.split() if m]
    wait_for_api(base,startup,req_timeout)
    if loop<=0:
        return 0 if reconcile_once(base,required,req_timeout) >= 0 else 1
    while True:
        try: reconcile_once(base,required,req_timeout)
        except Exception as e: print(f"[error] {e}",file=sys.stderr)
        time.sleep(loop)

if __name__=="__main__":
    sys.exit(main())

