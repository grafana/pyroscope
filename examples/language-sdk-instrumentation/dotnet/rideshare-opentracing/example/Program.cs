using System;
using System.Collections.Generic;
using System.IO;
using System.Threading;
using System.Net.Http;
using System.Collections;

using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Builder;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.DependencyInjection;

using Jaeger;
using Jaeger.Senders;
using Jaeger.Senders.Thrift;

using Pyroscope.OpenTracing;
using OpenTracing.Util;

namespace Example;

public static class Program
{
    private static readonly List<FileStream> Files = new();
    public static void Main(string[] args)
    {
        var builder = WebApplication.CreateBuilder(args);
        ILoggerFactory loggerFactory = LoggerFactory.Create(b => b.AddConsole());

        Configuration.SenderConfiguration.DefaultSenderResolver = new SenderResolver(loggerFactory).RegisterSenderFactory<ThriftSenderFactory>();
        var tracingConfig = Configuration.FromEnv(loggerFactory);
        var tracer = tracingConfig.GetTracer();
        GlobalTracer.Register(new PyroscopeTracer(tracer));

        builder.Services.AddOpenTracing();

        var app = builder.Build();

        for (int i = 0; i < 1024; i++)
        {
            Files.Add(File.Open("/dev/null", FileMode.Open, FileAccess.Read, FileShare.Read));
        }
        object globalLock = new();
        var strings = new List<string>();
        var orderService = new OrderService();
        var bikeService = new BikeService(orderService);
        var scooterService = new ScooterService(orderService);
        var carService = new CarService(orderService);

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

        app.Run();
    }
}
