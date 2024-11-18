package org.example.rideshare.car;

import org.example.rideshare.OrderService;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Service;

@Service
public class CarService {
    @Autowired
    OrderService orderService;

    public void orderCar(int searchRadius) {
        orderService.findNearestVehicle(searchRadius, "car");
    }

}
