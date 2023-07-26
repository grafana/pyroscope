package rideshare;

import io.opentelemetry.api.OpenTelemetry;
import io.opentelemetry.api.trace.Span;
import io.opentelemetry.api.trace.Tracer;
import io.opentelemetry.context.Scope;
import org.slf4j.MDC;
import rideshare.bike.BikeService;
import rideshare.car.CarService;
import rideshare.scooter.ScooterService;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;

import java.util.Map;

@RestController
public class RideShareController {
    Logger logger = LoggerFactory.getLogger(RideShareController.class);


    @Autowired
    CarService carService;

    @Autowired
    ScooterService scooterService;

    @Autowired
    OpenTelemetry otel;

    @Autowired
    BikeService bikeService;

    @Autowired
    Tracer tracer;



    @GetMapping("/bike")
    public String orderBike() {
        Span span = tracer.spanBuilder("orderBike")
                .startSpan();
        String res;
        try (Scope s = span.makeCurrent()){
            logger.info("orderBike");
            bikeService.orderBike(/* searchRadius */ 1);
            res = "<h1>Bike ordered</h1>";
        } catch (Exception th) {
            span.recordException(th);
            throw new RuntimeException(th);
        } finally {
            span.end();
        }
        return res;
    }

    @GetMapping("/scooter")
    public String orderScooter() {

        Span span = tracer.spanBuilder("orderScooter")
                .startSpan();
        String res;
        try (Scope s = span.makeCurrent()){
            logger.info("orderScooter");
            scooterService.orderScooter(/* searchRadius */ 2);
            res = "<h1>Scooter ordered</h1>";
        } catch (Exception e) {
            span.recordException(e);
            throw new RuntimeException(e);
        } finally {
            span.end();
        }
        return res;
    }

    @GetMapping("/car")
    public String orderCar() {
        Span span = tracer.spanBuilder("orderCar")
                .startSpan();
        String res;
        try (Scope s = span.makeCurrent()){
            logger.info("orderCar");
            carService.orderCar(/* searchRadius */ 3);
            res = "<h1>Car ordered</h1>";
        } catch (Exception e) {
            span.recordException(e);
            throw new RuntimeException(e);
        } finally {
            span.end();
        }
        return res;
    }

    @GetMapping("/")
    public String env() {

        Span span = tracer.spanBuilder("env")
                .startSpan();
        String res;
        try (Scope s = span.makeCurrent()) {
            logger.info("env");
            StringBuilder sb = new StringBuilder();
            for (Map.Entry<String, String> it : System.getenv().entrySet()) {
                sb.append(it.getKey()).append(" = ").append(it.getValue()).append("<br>\n");
            }
            res = sb.toString();
        } finally {
            span.end();
        }
        return res;
    }
}
