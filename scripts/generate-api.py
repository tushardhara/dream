"""Pinned local generation and byte-for-byte drift check; no remote plugins."""
import hashlib
import os
import pathlib
import subprocess
import sys
import tempfile
import urllib.request
import zipfile

ROOT = pathlib.Path(__file__).resolve().parent.parent
CACHE = pathlib.Path(os.environ.get('XDG_CACHE_HOME', pathlib.Path.home()/'.cache'))/'dream-tools'
PROTOC_VERSION = '36.1'
PROTOC_SHA = 'c4bc672d9d49214dc8cafdceadf4df92182d6ca8e3ec65a56b2d7de5602669b4'
PLUGINS = {
 'protoc-gen-go': 'google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12',
 'protoc-gen-go-grpc': 'google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.2',
 'protoc-gen-grpc-gateway': 'github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@v2.30.0',
 'protoc-gen-openapiv2': 'github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@v2.30.0',
}

def run(*args, **kw):
 return subprocess.run(args, check=True, timeout=180, **kw)

def tools():
 CACHE.mkdir(parents=True, exist_ok=True)
 compiler = CACHE/f'protoc-{PROTOC_VERSION}'/'bin/protoc'
 # Validate compiler from a verified upstream archive before every invocation.
 archive=CACHE/f'protoc-{PROTOC_VERSION}.zip'
 if not archive.exists():
  with urllib.request.urlopen(f'https://github.com/protocolbuffers/protobuf/releases/download/v{PROTOC_VERSION}/protoc-{PROTOC_VERSION}-linux-x86_64.zip', timeout=30) as r:
   raw=r.read(8<<20)
  if hashlib.sha256(raw).hexdigest()!=PROTOC_SHA: raise RuntimeError('compiler checksum mismatch')
  archive.write_bytes(raw)
 if hashlib.sha256(archive.read_bytes()).hexdigest()!=PROTOC_SHA: raise RuntimeError('cached compiler checksum mismatch')
 with zipfile.ZipFile(archive) as z:
  compiler.parent.mkdir(parents=True,exist_ok=True)
  compiler.write_bytes(z.read('bin/protoc'))
 compiler.chmod(0o755)
 env=dict(os.environ,GOBIN=str(CACHE))
 for name,module in PLUGINS.items():
  binary=CACHE/name
  expected=module.rsplit('@',1)
  info=subprocess.run(['go','version','-m',str(binary)],capture_output=True,text=True)
  lines=[line.split() for line in info.stdout.splitlines()]
  package_ok=any(parts==['path',expected[0]] for parts in lines)
  version_ok=any(len(parts)>=3 and parts[0]=='mod' and parts[2]==expected[1] for parts in lines)
  if info.returncode or not package_ok or not version_ok:
   run('go','install',module,env=env)
 return compiler

def main():
 compiler=tools()
 with tempfile.TemporaryDirectory(prefix='dream-generated-') as tmp:
  out=pathlib.Path(tmp);gen=out/'adapters/transport/gen';api=out/'api/openapi';gen.mkdir(parents=True);api.mkdir(parents=True)
  run(str(compiler),'-I',str(ROOT/'api/proto'),f'--go_out={gen}','--go_opt=paths=source_relative',f'--go-grpc_out={gen}','--go-grpc_opt=paths=source_relative',f'--grpc-gateway_out={gen}','--grpc-gateway_opt=paths=source_relative,generate_unbound_methods=true',f'--openapiv2_out={api}','--openapiv2_opt=generate_unbound_methods=true',str(ROOT/'api/proto/dream/v1/service.proto'),env=dict(os.environ,PATH=str(CACHE)+os.pathsep+os.environ['PATH']))
  paths=[p.relative_to(out) for p in out.rglob('*') if p.is_file()]
  actual={p.relative_to(ROOT) for directory in ['adapters/transport/gen','api/openapi'] for p in (ROOT/directory).rglob('*') if p.is_file()}
  if '--write' in sys.argv:
   for p in paths:
    (ROOT/p).parent.mkdir(parents=True,exist_ok=True);(ROOT/p).write_bytes((out/p).read_bytes())
  elif actual!=set(paths) or any(not (ROOT/p).exists() or (ROOT/p).read_bytes()!=(out/p).read_bytes() for p in paths):
   raise SystemExit('BLOCKED: generated API drift; run python3 scripts/generate-api.py --write')
 print('PASS: pinned protobuf/gRPC/gateway/OpenAPI regeneration')
if __name__=='__main__':main()
