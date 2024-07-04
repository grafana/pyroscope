import signal
import sys
import threading

import requests

running = True


def signal_handler(sig, frame):
  global running
  running = False


signal.signal(signal.SIGINT, signal_handler)
signal.signal(signal.SIGTERM, signal_handler)

dst = sys.argv[1]


def run():
  global running
  s = requests.Session()

  n = 0
  e = 0
  while running:
    try:
      r = s.get(dst)
      n += 1
    except KeyboardInterrupt:
      break
    except:
      e += 1
      pass

  print(f"Success: {n}, Error: {e}")


ts = []
for i in range(16):
  t = threading.Thread(target=run)
  t.start()
  ts.append(t)

for t in ts:
  t.join()
