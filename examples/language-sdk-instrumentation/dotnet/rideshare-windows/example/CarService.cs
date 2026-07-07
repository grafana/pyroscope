namespace Example;

internal class CarService
{
    private readonly OrderService _orderService;

    public CarService(OrderService orderService)
    {
        _orderService = orderService;
    }

    public void Order(int searchRadius)
    {
        _orderService.FindNearestVehicle(searchRadius, "car");
    }
}