package rideshare.bike;

import rideshare.OrderService;
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
