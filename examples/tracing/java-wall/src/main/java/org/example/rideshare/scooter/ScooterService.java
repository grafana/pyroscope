package org.example.rideshare.scooter;

import org.example.rideshare.OrderService;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Service;

@Service
public class ScooterService {
    @Autowired
    OrderService orderService;

    public void orderScooter(int searchRadius) {
        orderService.findNearestVehicle(searchRadius, "scooter");
    }

}
