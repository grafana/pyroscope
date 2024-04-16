using OpenTracing;
using OpenTracing.Util;

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
        using IScope scope = GlobalTracer.Instance.BuildSpan("Order").StartActive(finishSpanOnDispose: true);
        scope.Span.SetTag("type", "scooter");
        for (long i = 0; i < 2000000000; i++)
        {
        }
        OrderInternal(searchRadius);
        DoSomeOtherWork();
    }

    private void OrderInternal(int searchRadius)
    {
        using IScope scope = GlobalTracer.Instance.BuildSpan("OrderInternal").StartActive(finishSpanOnDispose: true);
        _orderService.FindNearestVehicle(searchRadius, "scooter");
    }

    private void DoSomeOtherWork()
    {
        for (long i = 0; i < 1000000000; i++)
        {
        }
    }
}