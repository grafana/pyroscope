import time
import threading
from a_module.foo import foo
from b_module.bar import bar

ITERATIONS = 251678

if __name__ == "__main__":

  print(f'yoyoy Application 1 started')
  for i in range(ITERATIONS):
    foo(75000)
    bar(25000)
  print(f'yoyoy Application 1 ended')

  print(f'Switching fast and slow around.....')
  time.sleep(20)
  print(f'Switching fast and slow around.....')

  print(f'yoyoy Application 2 started')
  for i in range(ITERATIONS):
    bar(25000)
    foo(8000)
  print(f'yoyoy Application 2 ended')
