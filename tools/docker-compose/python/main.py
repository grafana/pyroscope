import time

# import profiling modules
from pypprof.net_http import start_pprof_server
import mprofile

def prime_number_from_1_to(to=100):
    prime_numbers = []
    for i in range(1, to):
        if i > 1:
            for j in range(2, i):
                if divisible(i, j):
                    break
            else:
                prime_numbers.append(i)
    return prime_numbers

def divisible(i, j):
    return i % j == 0


def main():
    # start memory profiling
    mprofile.start(sample_rate=128 * 1024)

    # enable pprof http server
    start_pprof_server(host='0.0.0.0', port=8080)

    to = 500
    while True:
        result = prime_number_from_1_to(to)
        print("there are %d prime numbers from 1 to %d" % (len(result), to))
        time.sleep(0.5)

if __name__ == "__main__":
    main()
