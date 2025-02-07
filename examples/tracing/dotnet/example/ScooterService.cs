using System.Diagnostics;

namespace Example;

internal class ScooterService
{
    private static readonly ActivitySource CustomActivity = new(Program.CustomActivitySourceName);

    private readonly OrderService _orderService;

    public ScooterService(OrderService orderService)
    {
        _orderService = orderService;
    }

    public void Order(int searchRadius)
    {
        using var activity = CustomActivity.StartActivity("OrderScooter");
        activity?.SetTag("type", "scooter");
        for (long i = 0; i < 2000000000; i++)
        {
        }
        OrderInternal(searchRadius);
        DoSomeOtherWork();
    }

    private void OrderInternal(int searchRadius)
    {
        using var activity = CustomActivity.StartActivity("OrderScooterInternal");
        _orderService.FindNearestVehicle(searchRadius, "scooter");
    }

    private void DoSomeOtherWork()
    {
        for (long i = 0; i < 1000000000; i++)
        {
        }
    }
}
