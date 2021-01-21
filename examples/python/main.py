import time
import threading
from a_module.fast_file import fast_function
from b_module.slow_file import slow_function

DURATION = 60 * 10 # ten minutes

if __name__ == "__main__":
  t_start = time.time()
  t_end = time.time() + DURATION

  print(f'yoyoy Application started at: {t_start}')
  while time.time() < t_end:
    threads = list()
    fast_thread = threading.Thread(target=fast_function, args=(25000,))
    slow_thread = threading.Thread(target=slow_function, args=(8000,))

    threads.append(fast_thread)
    fast_thread.start()

    threads.append(slow_thread)
    slow_thread.start()

    for index, thread in enumerate(threads):
        # print("Main    : before joining thread %d.", index)
        thread.join()
        # print("Main    : thread %d done", index)

  print(f'yoyoy Application ended at: {t_end}')

  print(f'Switching fast and slow around.....')
  time.sleep(60)
  print(f'Switching fast and slow around.....')

  t_start = time.time()
  t_end = time.time() + DURATION

  print(f'yoyoy Application started at: {t_start}')
  while time.time() < t_end:
    threads = list()
    fast_thread = threading.Thread(target=fast_function, args=(25000,))
    slow_thread = threading.Thread(target=slow_function, args=(75000,))

    threads.append(fast_thread)
    fast_thread.start()

    threads.append(slow_thread)
    slow_thread.start()

    for index, thread in enumerate(threads):
        # print("Main    : before joining thread %d.", index)
        thread.join()
        # print("Main    : thread %d done", index)

  print(f'yoyoy Application ended at: {t_end}')




