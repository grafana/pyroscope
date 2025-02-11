namespace Example;

internal class BikeService
{
    private readonly OrderService _orderService;

    public BikeService(OrderService orderService)
    {
        _orderService = orderService;
    }

    public void Order(int searchRadius)
    {
        _orderService.FindNearestVehicle(searchRadius, "bike");
    }
}