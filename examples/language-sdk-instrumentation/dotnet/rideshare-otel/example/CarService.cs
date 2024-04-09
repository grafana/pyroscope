using System;
using System.Threading;
using Microsoft.Extensions.Logging;

namespace Example;

internal class CarService
{
    private readonly OrderService _orderService;
    private readonly ILogger _logger;

    public CarService(OrderService orderService, ILoggerFactory loggerFactory)
    {
        _orderService = orderService;
        _logger = loggerFactory.CreateLogger("car");
    }

    public void Order(int searchRadius)
    {
        _logger.LogInformation("CarService: Current thread ID: {0}", Environment.CurrentManagedThreadId);
        _orderService.FindNearestVehicle(searchRadius, "car");
    }
}