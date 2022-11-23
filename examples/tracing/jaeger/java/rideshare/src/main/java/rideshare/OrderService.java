package rideshare;

import io.opentelemetry.api.trace.Span;
import io.opentelemetry.api.trace.Tracer;
import io.opentelemetry.context.Scope;

import io.otel.pyroscope.shadow.labels.LabelsSet;
import io.otel.pyroscope.shadow.labels.Pyroscope;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Service;

import java.time.Duration;
import java.time.Instant;
import java.time.ZoneOffset;
import java.time.temporal.ChronoUnit;
import java.util.concurrent.atomic.AtomicLong;

@Service
public class OrderService {

    public static final Duration OP_DURATION = Duration.of(200, ChronoUnit.MILLIS);


    @Autowired
    Tracer tracer;
    Logger logger = LoggerFactory.getLogger(OrderService.class);

    public synchronized void findNearestVehicle(int searchRadius, String vehicle) {
        logger.info("findNearestVehicle");
        Span span = tracer.spanBuilder("findNearestVehicle").startSpan();
        try (Scope s = span.makeCurrent()){
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
        } finally {
            span.end();
        }
    }

    private void checkDriverAvailability(int searchRadius) {
        logger.info("checkDriverAvailability");

        Span span = tracer.spanBuilder("checkDriverAvailability").startSpan();
        try (Scope s = span.makeCurrent()){
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
            if ("eu-north".equals(System.getenv("REGION")) && force_mutex_lock) {
                mutexLock(searchRadius);
            }
        } finally {
            span.end();
        }
    }

    private void mutexLock(int searchRadius) {
        logger.info("mutexLock");

        Span span = tracer.spanBuilder("mutexLock").startSpan();
        try (Scope s = span.makeCurrent()){
            AtomicLong i = new AtomicLong();
            Instant end = Instant.now()
                    .plus(OP_DURATION.multipliedBy(30L * searchRadius));
            while (Instant.now().compareTo(end) <= 0) {
                i.incrementAndGet();
            }
        } finally {
            span.end();
        }
    }

}
