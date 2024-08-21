import java.util.concurrent.locks.Lock;
import java.util.concurrent.locks.ReentrantLock;

class TransactionProcessor implements Runnable {
    private Lock lock;

    public TransactionProcessor(Lock lock) {
        this.lock = lock;
    }

    public void run() {
        while (true) {
            this.lock.lock();
            try {
                processTransaction(1000); // Simulating a transaction with a CPU-intensive task
            } finally {
                this.lock.unlock();
            }
        }
    }

    public static void processTransaction(int amount) {
        for (int i = 0; i < amount; i++) {
            calculateInterest(1.05, 1000000);
        }
    }

    public static double calculateInterest(double rate, int iterations) {
        double result = 0;
        for (int i = 0; i < iterations; i++) {
            result += Math.pow(rate, i) / (i + 1);
        }
        return result;
    }
}

class AccountStatementGenerator implements Runnable {
    private Lock lock;

    public AccountStatementGenerator(Lock lock) {
        this.lock = lock;
    }

    public void run() {
        while (true) {
            this.lock.lock();
            try {
                generateStatement(100); // Simulating account statement generation
            } finally {
                this.lock.unlock();
            }
        }
    }

    public static void generateStatement(int accounts) {
        for (int i = 0; i < accounts; i++) {
            calculateInterest(1.03, 500000); // Less intensive task
        }
    }

    public static double calculateInterest(double rate, int iterations) {
        double result = 0;
        for (int i = 0; i < iterations; i++) {
            result += Math.pow(rate, i) / (i + 1);
        }
        return result;
    }
}

class Main {
    public static void main(String[] args) {
        Lock transactionLock = new ReentrantLock();
        Lock statementLock = new ReentrantLock();

        Runnable transactionProcessor = new TransactionProcessor(transactionLock);
        Runnable statementGenerator = new AccountStatementGenerator(statementLock);

        new Thread(transactionProcessor).start();
        new Thread(statementGenerator).start();

        while (true) {
            checkAccountBalance(100);
        }
    }

    public static void checkAccountBalance(int accounts) {
        for (int i = 0; i < accounts; i++) {
            simulateCPULoad(200000);
        }
    }

    public static void simulateCPULoad(int iterations) {
        double result = 0;
        for (int i = 0; i < iterations; i++) {
            result += Math.sin(i) * Math.cos(i);
        }
    }
}
