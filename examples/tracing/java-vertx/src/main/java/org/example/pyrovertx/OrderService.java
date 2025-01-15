package org.example.pyrovertx;

import java.time.Duration;
import java.time.Instant;
import java.time.ZoneOffset;
import java.time.temporal.ChronoUnit;
import java.util.concurrent.atomic.AtomicLong;

public class OrderService {

    public static final OrderService INSTANCE = new OrderService();

    public static final Duration OP_DURATION = Duration.of(200, ChronoUnit.MILLIS);

    public synchronized void findNearestVehicle(int searchRadius, String vehicle) {
        AtomicLong i = new AtomicLong();
        Instant end = Instant.now()
                .plus(OP_DURATION.multipliedBy(searchRadius));
        while (Instant.now().compareTo(end) <= 0) {
            i.incrementAndGet();
        }

        if (vehicle.equals("car")) {
            checkDriverAvailability(searchRadius);
        }
    }

    private void checkDriverAvailability(int searchRadius) {
        AtomicLong i = new AtomicLong();
        Instant end = Instant.now()
                .plus(OP_DURATION.multipliedBy(searchRadius));
        while (Instant.now().compareTo(end) <= 0) {
            i.incrementAndGet();
        }
        // Every other minute this will artificially create make requests in eu-north region slow
        // this is just for demonstration purposes to show how performance impacts show up in the
        // flamegraph
        Instant.now().atZone(ZoneOffset.UTC);
        if (System.getenv("REGION").equals("eu-north")) {
            mutexLock(searchRadius);
        }
    }

    private void mutexLock(int searchRadius) {
        AtomicLong i = new AtomicLong();
        Instant end = Instant.now()
                .plus(OP_DURATION.multipliedBy(30L * searchRadius));
        while (Instant.now().compareTo(end) <= 0) {
            i.incrementAndGet();
        }
    }

}
