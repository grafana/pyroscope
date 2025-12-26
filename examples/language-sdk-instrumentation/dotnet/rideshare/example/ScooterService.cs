using System;
using System.Diagnostics;

namespace Example;

internal class ScooterService
{
    private static readonly ActivitySource CustomActivity = new(Program.CustomActivitySourceName);
    public static string? sss = null;
    public static int ExceptionCount = 0;
    private readonly OrderService _orderService;

    public ScooterService(OrderService orderService)
    {
        _orderService = orderService;
    }

    public void Order(int searchRadius)
    {
        using var activity = CustomActivity.StartActivity("OrderScooter");
        activity?.SetTag("type", "scooter");
        bool p = false;
        for (long i = 0; i < 200; i++)
        {
            try
            {
                ExceptionCount += ScooterService.sss.ToLower().Length;
            }
            catch
            {
                if (!p)
                {
                    Console.WriteLine("Exception occurred in ScooterService.Order");
                    p = true;
                }
                ScooterService.ExceptionCount++;
            }
        }

        // OrderInternal(searchRadius);
        // DoSomeOtherWork();
    }

    private void OrderInternal(int searchRadius)
    {
        using var activity = CustomActivity.StartActivity("OrderScooterInternal");
        // _orderService.FindNearestVehicle(searchRadius, "scooter");
    }

    private void DoSomeOtherWork()
    {
        bool p = false;
        // for (long i = 0; i < 10; i++)
        {
            try
            {
                ExceptionCount += ScooterService.sss.ToLower().Length;
            }
            catch
            {
                if (!p)
                {
                    Console.WriteLine("Exception occurred in ScooterService.DoSomeOtherWork");
                    p = true;
                }
                ScooterService.ExceptionCount++;
            }
        }
    }
}
