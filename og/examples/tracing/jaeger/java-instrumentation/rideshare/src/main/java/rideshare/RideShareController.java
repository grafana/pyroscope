package rideshare;

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
    BikeService bikeService;

    @GetMapping("/bike")
    public String orderBike() {
        logger.info("orderBike");
        bikeService.orderBike(/* searchRadius */ 1);
        return "<h1>Bike ordered</h1>";
    }

    @GetMapping("/scooter")
    public String orderScooter() {

        logger.info("orderScooter");
        scooterService.orderScooter(/* searchRadius */ 2);
        return "<h1>Scooter ordered</h1>";

    }

    @GetMapping("/car")
    public String orderCar() {
        logger.info("orderCar");
        carService.orderCar(/* searchRadius */ 3);
        return "<h1>Car ordered</h1>";
    }

    @GetMapping("/")
    public String env() {
        logger.info("env");
        StringBuilder sb = new StringBuilder();
        for (Map.Entry<String, String> it : System.getenv().entrySet()) {
            sb.append(it.getKey()).append(" = ").append(it.getValue()).append("<br>\n");
        }
        return sb.toString();
    }
    @GetMapping("/properties")
    public String properties() {
        logger.info("properties");
        StringBuilder sb = new StringBuilder();
        for (Map.Entry<Object, Object> it : System.getProperties().entrySet()) {
            sb.append(it.getKey()).append(" = ").append(it.getValue()).append("<br>\n");
        }
        return sb.toString();
    }
}
