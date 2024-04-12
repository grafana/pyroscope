package org.example.rideshare;

import io.pyroscope.labels.LabelsSet;
import io.pyroscope.labels.Pyroscope;
import org.springframework.stereotype.Service;

import java.time.Duration;
import java.time.Instant;
import java.time.ZoneOffset;
import java.time.temporal.ChronoUnit;
import java.util.concurrent.atomic.AtomicLong;

@Service
public class OrderService {

    public static final Duration OP_DURATION = Duration.of(200, ChronoUnit.MILLIS);

    public synchronized void findNearestVehicle(int searchRadius, String vehicle) {
        Pyroscope.LabelsWrapper.run(new LabelsSet("vehicle", vehicle), () -> {
            AtomicLong i = new AtomicLong();
            Instant end = Instant.now()
                    .plus(OP_DURATION.multipliedBy(searchRadius));
            while (Instant.now().compareTo(end) <= 0) {
                i.incrementAndGet();
            }

            if (vehicle.equals("car")) {
                checkDriverAvailability(searchRadius);
            }
        });
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
        boolean force_mutex_lock = Instant.now().atZone(ZoneOffset.UTC).getMinute() % 2 == 0;
        if (System.getenv("REGION").equals("eu-north") && force_mutex_lock) {
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
