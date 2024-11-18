package org.example.rideshare.bike;

import org.example.rideshare.OrderService;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Service;

@Service
public class BikeService {
    @Autowired
    OrderService orderService;

    public void orderBike(int searchRadius) {
        orderService.findNearestVehicle(searchRadius, "bike");
    }
}
