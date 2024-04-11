using System;
using System.Collections.Generic;
using System.IO;
using System.Threading;
using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Builder;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.DependencyInjection;
using System.Collections;

using Jaeger;
using Jaeger.Senders;
using Jaeger.Senders.Thrift;
using Jaeger.Reporters;
using OpenTracing.Contrib.NetCore.Configuration;
using OpenTracing;
using System.Net.Http;

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

        builder.Services.AddOpenTracingCoreServices(b => {
                b.AddAspNetCore()
                    .ConfigureAspNetCore(options => {
                        options.Hosting.OnRequest = (span, request) => {
                            var spanId = span.Context.SpanId;
                            var spanIdLong = Convert.ToUInt64(spanId.ToUpper(), 16);
                            Pyroscope.Profiler.Instance.SetProfileId(spanIdLong);
                            span.SetTag("pyroscope.profile.id", spanId);
                        };
                    });
            });
        builder.Services.AddSingleton(tracer);

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
        
        
        app.MapGet("/pyroscope/cpu", (HttpRequest request) =>
        {
            var enable = request.Query["enable"] == "true";
            Pyroscope.Profiler.Instance.SetCPUTrackingEnabled(enable);
            return "OK";
        });
        app.MapGet("/pyroscope/allocation", (HttpRequest request) =>
        {
            var enable = request.Query["enable"] == "true";
            Pyroscope.Profiler.Instance.SetAllocationTrackingEnabled(enable);
            return "OK";
        });
        app.MapGet("/pyroscope/contention", (HttpRequest request) =>
        {
            var enable = request.Query["enable"] == "true";
            Pyroscope.Profiler.Instance.SetContentionTrackingEnabled(enable);
            return "OK";
        });
        app.MapGet("/pyroscope/exception", (HttpRequest request) =>
        {
            var enable = request.Query["enable"] == "true";
            Pyroscope.Profiler.Instance.SetExceptionTrackingEnabled(enable);
            return "OK";
        });
        
        
        app.MapGet("/playground/allocation", (HttpRequest request) =>
        {
            var strings = new List<string>();
            for (var i = 0; i < 10000; i++)
            {
                strings.Add("foobar" + i);
            }

            return "OK";
        });
        app.MapGet("/playground/contention", (HttpRequest request) =>
        {
            for (var i = 0; i < 100; i++)
            {
                lock (globalLock)
                {
                    Thread.Sleep(10);
                }
            }
            return "OK";
        });
        app.MapGet("/playground/exception", (HttpRequest request) =>
        {
            for (var i = 0; i < 1000; i++)
            {
                try
                {
                    throw new Exception("foobar" + i);
                }
                catch (Exception)
                {
                }
            }
            return "OK";
        });
        app.MapGet("/playground/leak", (HttpRequest request) =>
        {
            for (var i = 0; i < 1000; i++)
            {
                strings.Add("leak " + i);
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