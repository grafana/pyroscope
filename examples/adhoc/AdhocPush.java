import java.util.Random;

public class AdhocPush {
    public static boolean isPrime(long n) {
        for (long i = 2; i <= n; i++) {
            if (i * i > n) return true;
            if (n % i == 0) return false;
        }
        return false;
    }

    public static long slow(long n) {
        long sum = 0;
        for (long i = 0; i <= n; i++) {
            sum += i;
        }
        return sum;
    }

    public static long fast(long n) {
        long sum = 0;
        long root = (long) Math.sqrt(n);
        for (long a = 1; a <= n; a += root) {
            long b = Math.min(a + root -1, n);
            sum += (b - a + 1) * (a + b) / 2;
        }
        return sum;
    }

    public static void run() {
        Random r = new Random();
        long base = r.nextInt(1000000) + 1;
        for (long i = 0; i < 40000000; i++) {
            long n = r.nextInt(10000) + 1;
            if (isPrime(base + i))
                slow(n);
            else
                fast(n);
        }
    }

    public static void main(String[] args) {
        run();
    }
}
