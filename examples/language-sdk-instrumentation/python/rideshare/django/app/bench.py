import re
import shutil
import subprocess
import threading
import time

import psutil

gunicorn = shutil.which('gunicorn')
go = shutil.which('go')
print(gunicorn)
print(go)

nworkers = 12
duration = 10000
ntests = 3


def run_one_test(use_pyroscope: bool, loadgen: bool, sample_rate: int):
  global nworkers
  global duration
  print(
    f"    pyroscope = {use_pyroscope}\n    loadgen = {loadgen}\n    nworkers = {nworkers}\n    duration_seconds = {duration}\n    sample_rate = {sample_rate}")
  command = [gunicorn, f'--workers={nworkers}', 'hello_django.wsgi']

  process = subprocess.Popen(command,
                             #                                cwd=cwd,
                             stdout=subprocess.PIPE,
                             stderr=subprocess.PIPE,
                             env={
                               'DEBUG': '0',
                               'SECRET_KEY': 'toor',
                               'DJANGO_ALLOWED_HOSTS': 'localhost',
                               # 'RUST_BACKTRACE': '1',
                               'RUST_BACKTRACE': 'full',
                               # 'PYTHONUNBUFFERED': '1',
                               'PYROSCOPE_BENCHMARK': 'true' if use_pyroscope else 'false',
                               'PYROSCOPE_BENCHMARK_SAMPLERATE': str(sample_rate),
                             })

  import queue

  def reader(pipe, queue):
    res = b''
    try:
      with pipe:
        for line in iter(pipe.readline, b''):
          res += line.strip() + b'\n'
    finally:
      queue.put(res)

  q = queue.Queue()
  threading.Thread(target=reader, args=[process.stdout, q]).start()
  threading.Thread(target=reader, args=[process.stderr, q]).start()




  p = psutil.Process(process.pid)

  print('waiting for workers')
  while True:
    children = p.children(recursive=False)
    if len(children) == nworkers:
      break
    time.sleep(0.1)

  if loadgen:
    print('starting loadgen')
    # loadgenproc = subprocess.Popen(['python3', 'bench_loadgen.py', 'http://localhost:8000'])
    # loadgenproc = subprocess.Popen([go, 'run', 'bench_loadgen.go', '-dst=http://localhost:8000', f'-nworkers={nworkers}'])
    loadgenproc = subprocess.Popen([ './bench_loadgen', '-dst=http://localhost:8000', f'-nworkers={nworkers}'])

  print('workers are ready')
  print('waiting for 5 seconds just in case')
  time.sleep(5)
  print('starting measurement')

  def get_time():
    user = 0
    system = 0
    for child in (children + [p]):
      times = child.cpu_times()
      user += times.user
      system += times.system
    return (user, system)

  def ptime(name, u, s):
    print(f" {name:10s} user: {u:10.1f}, system: {s:10.1f}")



  user, system = get_time()
  ptime('t1', user, system)

  time.sleep(duration)

  user2, system2 = get_time()
  ptime('t2', user2, system2)

  process.terminate()

  process.wait()

  out1, out2 = q.get(), q.get()

  panics = re.findall(b' panicked at ', out1 + out2)
  npanics = len(panics)
  if len(panics) > 0:
    print(out1.decode())
    print(out2.decode())

  ptime('diff', user2 - user, system2 - system)
  print(f"  panics: {npanics}")

  if loadgen:
    print("waiting for loadgen to finish")
    loadgenproc.terminate()
    print(loadgenproc.wait())
    print("loadgen finished")

  return user2 - user, system2 - system, npanics



all_test_results = []


def run_multiple_test(args):
  pyroscope_res = []
  for i in range(ntests):
    print(f'\n============== running {i+1} of {ntests} =====================')
    pyroscope_res.append(run_one_test(**args))

  name = f'pyroscope = {str(args["use_pyroscope"]):6s} loadgen = {str(args["loadgen"]):6s} samplerate = {str(args["sample_rate"]):6s} '
  all_test_results.append((name, pyroscope_res))


def pres(res, name):
  npanics = sum(p for _, _, p in res)
  print(f"{name:80s}    user: {sum(u for u, _, _ in res) / ntests:10.1f}, system: {sum(s for _, s, _ in res) / ntests:10.1f} panics: {npanics:3d}")


run_multiple_test({'use_pyroscope': True, 'sample_rate': 100, 'loadgen': False, })
# run_multiple_test({'use_pyroscope': True, 'sample_rate': 17, 'loadgen': False, })
# run_multiple_test({'use_pyroscope': True, 'sample_rate': 100, 'loadgen': True, })
# run_multiple_test({'use_pyroscope': True, 'sample_rate': 17, 'loadgen': True, })
# run_multiple_test({'use_pyroscope': False, 'sample_rate': 0, 'loadgen': False, })
# run_multiple_test({'use_pyroscope': False, 'sample_rate': 0, 'loadgen': True, })

print('============================== RESULTS ==============================')
print(f'ran ntests = {ntests} nworkers = {nworkers} duration = {duration}')
for result in all_test_results:
  pres(result[1], result[0])
