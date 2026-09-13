"""Opt-in engineering negative controls; use an exclusive implementation worktree.
Runs green baselines, removes one guard at a time, requires a test assertion failure,
and restores original bytes in finally. No merges, live data or permissions changes.
"""
import pathlib,subprocess
root=pathlib.Path(__file__).resolve().parents[1]
logs=root/'bin'/'evaluation-mutations'
logs.mkdir(parents=True,exist_ok=True)
subprocess.run(['go','test','-count=1','./evals','./cmd/hws-eval','./adapters/evaluation'],cwd=root,check=True,timeout=120)
subprocess.run(['python3','scripts/evaluation-check.py'],cwd=root,check=True,timeout=300)
probes=[
 ('split-isolation','evals/dataset.go','previous != c.Split','previous != c.Split && false',['go','test','-count=1','./evals','-run','^TestDatasetLeakageAndConsentNegatives$']),
 ('proper-score','evals/runner.go','math.Pow(mean-y, 2)','math.Pow(mean-y, 2)*0',['go','test','-count=1','./evals','-run','^TestFrozenOfflineReportAndCalibration$']),
 ('owner-signature','evals/promotion.go','return e == nil && ed25519.Verify(public, message, raw)','return e == nil && (len(raw)>=0 || ed25519.Verify(public,message,raw))',['go','test','-count=1','./evals','-run','^TestPromotionRequiresSeparateOwnerSignaturesAndHoldout$']),
 ('request-binding','evals/runner.go','p.RequestHash != requestHash','false && p.RequestHash != requestHash',['go','test','-count=1','./evals','-run','^TestInvalidGeneratorAndNoData$']),
 ('actor-outage','simulator/experiment/generator.go','owned.Current.Outage || owned.Current.Horizon','false && owned.Current.Outage || owned.Current.Horizon',['go','test','-count=1','./evals','-run','^TestActualVariantsAndConditionalRollForward$']),
 ('file-permissions','cmd/hws-eval/main.go','info.Mode().Perm()&0077 != 0','false && info.Mode().Perm()&0077 != 0',['go','test','-count=1','./cmd/hws-eval','-run','^TestPrivateEvaluatorFiles$']),
]
probes += [
 ('container-network','adapters/evaluation/container.go','"--network=none", ','',['python3','scripts/evaluation-check.py']),
 ('container-readonly','adapters/evaluation/container.go','"--read-only", ','',['python3','scripts/evaluation-check.py']),
]
for name,file,old,new,command in probes:
 path=root/file;original=path.read_text()
 if original.count(old)!=1:raise RuntimeError(name+' expected exactly one source guard')
 try:
  path.write_text(original.replace(old,new))
  result=subprocess.run(command,cwd=root,text=True,capture_output=True,timeout=300)
  output=result.stdout+result.stderr
  (logs/(name+'.log')).write_text(output)
  if result.returncode==0 or '--- FAIL:' not in output:raise RuntimeError(name+' not caught by test assertion: '+output[-800:])
  print('CAUGHT '+name,flush=True)
 finally:path.write_text(original)

subprocess.run(['go','test','-count=1','./evals','./cmd/hws-eval','./adapters/evaluation'],cwd=root,check=True,timeout=120)
print('All eight guards restored; focused suite green.')
