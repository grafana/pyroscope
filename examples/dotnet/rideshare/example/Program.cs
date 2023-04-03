using System;
using System.Collections.Generic;
using System.Threading;
using Microsoft.AspNetCore.Http;

namespace Example;

using System.Collections;
using Microsoft.AspNetCore.Builder;

public static class Program
{
    public static void Main(string[] args)
    {
        object _lock = new();

        var orderService = new OrderService();
        var bikeService = new BikeService(orderService);
        var scooterService = new ScooterService(orderService);
        var carService = new CarService(orderService);

        var app = WebApplication.CreateBuilder(args).Build();
        app.MapGet("/bike", () =>
        {
            bikeService.Order(1);
            return "Bike ordered";
        });
        app.MapGet("/scooter", () =>
        {
            scooterService.Order(2);
            return "Scooter ordered";
        });

        app.MapGet("/car", () =>
        {
            carService.Order(3);
            return "Car ordered";
        });
        app.MapGet("/debug/sampler", (HttpRequest request) =>
        {
            bool enable = request.Query["enable"] == "true";
            Pyroscope.Profiler.Instance.SetStackSamplerEnabled(enable);
            return "OK";
        });
        app.MapGet("/debug/allocation", (HttpRequest request) =>
        {
            bool enable = request.Query["enable"] == "true";
            Pyroscope.Profiler.Instance.SetAllocationTrackingEnabled(enable);
            return "OK";
        });
        app.MapGet("/debug/contention", (HttpRequest request) =>
        {
            bool enable = request.Query["enable"] == "true";
            Pyroscope.Profiler.Instance.SetContentionTrackingEnabled(enable);
            return "OK";
        });
        app.MapGet("/pg/allocate", (HttpRequest request) =>
        {
            List<String> ss = new List<string>();
            for (int i = 0; i < 10000; i++)
            {
                ss.Add("foobar" + i);
            }

            return "OK";
        });
        app.MapGet("/pg/exception", (HttpRequest request) =>
        {
            for (int i = 0; i < 1000; i++)
            {
                try
                {
                    throw new Exception("foobar" + i);
                }
                catch (Exception e)
                {
                }
            }
            return "OK";
        });
        app.MapGet("/pg/contention", (HttpRequest request) =>
        {
            for (int i = 0; i < 100; i++)
            {
                lock (_lock)
                {
                    Thread.Sleep(10);
                }
            }
            return "OK";
        });
        app.MapGet("/", () =>
        {
            string env = "";
            foreach (DictionaryEntry e in System.Environment.GetEnvironmentVariables())
            {
                env += e.Key + " = " + e.Value + "<br>\n";
            }
            return env;
        });

        app.Run();
    }
}