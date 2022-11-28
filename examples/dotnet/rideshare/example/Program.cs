namespace Example;


using System.Collections;
using Microsoft.AspNetCore.Builder;



public static class Program
{
    public static void Main(string[] args)
    {
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



