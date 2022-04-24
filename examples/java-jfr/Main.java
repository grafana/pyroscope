import java.util.concurrent.locks.Lock;
import java.util.concurrent.locks.ReentrantLock;

class MyRunnable implements Runnable {
    private Lock lock;

    public MyRunnable(Lock lock) {
        this.lock = lock;
    }

    public static long fib(long n) {
        if (n < 2)
            return n;
        return fib(n-1) + fib(n-2);
    }

    public void run() {
        while (true) {
            this.lock.lock();
            try {
                fib(40);
            } finally {
                this.lock.unlock();
            }
        }
    }
}

class Main {
    public static long fib(long n) {
        if (n < 2)
            return n;
        return fib(n-1) + fib(n-2);
    }

    public static void main(String[] args) {
        Lock l = new ReentrantLock();
        Runnable r = new MyRunnable(l);
        new Thread(r).start();

        while (true) {
            l.lock();
            try {
                fib(40);
            } finally {
                l.unlock();
            }
        }
    }
}

