import java.util.concurrent.*;

public class FastSlow {

    public static long fib(int n) {
        if (n < 2) {
            return n;
        }
        return fib(n - 1) + fib(n - 2);
    }

    public static void main(String[] args) throws ExecutionException, InterruptedException {
        ExecutorService e = Executors.newSingleThreadExecutor();
        e.submit(() -> {
            while (true) {
                fib(26);
                fib(24);
            }
        });
    }
}