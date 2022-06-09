package org.example.rideshare;

import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;

import java.util.Map;

@RestController
public class RideShareController {

    @Autowired
    OrderService service;

    @GetMapping("/bike")
    public String orderBike() {
        service.orderBike(/* searchRadius */ 1);
        return "<h1>Bike ordered</h1>";
    }

    @GetMapping("/scooter")
    public String orderScooter() {
        service.orderScooter(/* searchRadius */ 2);
        return "<h1>Scooter ordered</h1>";
    }

    @GetMapping("/car")
    public String orderCar() {
        service.orderCar(/* searchRadius */ 3);
        return "<h1>Car ordered</h1>";
    }

    @GetMapping("/")
    public String env() {
        StringBuilder sb = new StringBuilder();
        for (Map.Entry<String, String> it : System.getenv().entrySet()) {
            sb.append(it.getKey()).append(" = ").append(it.getValue()).append("<br>\n");
        }
        return sb.toString();
    }
}
