namespace Example;

internal class ScooterService
{
    private readonly OrderService _orderService;

    public ScooterService(OrderService orderService)
    {
        _orderService = orderService;
    }

    public void Order(int searchRadius)
    {
        _orderService.FindNearestVehicle(searchRadius, "scooter");
    }
}