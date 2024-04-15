using System.Diagnostics;

namespace Example;

internal class ScooterService
{
    public static readonly ActivitySource MyActivity = new("Example.ScooterService");

    private readonly OrderService _orderService;

    public ScooterService(OrderService orderService)
    {
        _orderService = orderService;
    }

    public void Order(int searchRadius)
    {
        using var activity = MyActivity.StartActivity("OrderScooter");
        activity?.SetTag("type", "scooter");
        for (long i = 0; i < 2000000000; i++)
        {
        }
        OrderInternal(searchRadius);
        SomeOtherWork();
    }

    private void OrderInternal(int searchRadius)
    {
        using var activity = MyActivity.StartActivity("OrderScooterInternal");
        _orderService.FindNearestVehicle(searchRadius, "scooter");
    }

    private void SomeOtherWork()
    {
        for (long i = 0; i < 1000000000; i++)
        {
        }
    }
}