import time
import threading
from a_module.fast_file import fast_function
from b_module.slow_file import slow_function

ITERATIONS = 20000

if __name__ == "__main__":
  print(f'yoyoy Application 1 started')
  for i in range(ITERATIONS):
    fast_function(25000)
    slow_function(8000)
  print(f'yoyoy Application 1 ended')

  print(f'Switching fast and slow around.....')
  time.sleep(20)
  print(f'Switching fast and slow around.....')

  print(f'yoyoy Application 2 started')
  for i in range(ITERATIONS):
    fast_function(25000)
    slow_function(75000)
  print(f'yoyoy Application 2 ended')
