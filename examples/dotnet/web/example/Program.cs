using Microsoft.AspNetCore;
using Microsoft.AspNetCore.Builder;
using Microsoft.AspNetCore.Hosting;
using Microsoft.AspNetCore.Http;

public static class Program
{
    public static void Main()
    {
        WebHost.CreateDefaultBuilder()
            .Configure(app =>
            {
                app.UseRouting();
                app.UseEndpoints(endpoints =>
                {
                    endpoints.MapGet("/", async context =>
                    {
                        var j = 0;
                        for (var i = 0; i < 100000000; i++) j++;
                        await context.Response.WriteAsync("Hello!");
                    });
                });
            })
            .Build()
            .Run();
    }
}