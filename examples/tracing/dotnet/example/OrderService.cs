using System;

namespace Example;

internal class OrderService
{
    public void FindNearestVehicle(long searchRadius, string vehicle)
    {
        lock (_lock)
        {
            var labels = Pyroscope.LabelSet.Empty.BuildUpon()
                .Add("vehicle", vehicle)
                .Build();
            Pyroscope.LabelsWrapper.Do(labels, () =>
            {
                for (long i = 0; i < searchRadius * 1000000000; i++)
                {
                }

                if (vehicle.Equals("car"))
                {
                    CheckDriverAvailability(labels, searchRadius);
                }
            });
        }
    }

    private readonly object _lock = new();

    private static void CheckDriverAvailability(Pyroscope.LabelSet ctx, long searchRadius)
    {
        var region = System.Environment.GetEnvironmentVariable("REGION") ?? "unknown_region";
        ctx = ctx.BuildUpon()
            .Add("driver_region", region)
            .Build();
        Pyroscope.LabelsWrapper.Do(ctx, () =>
        {
            for (long i = 0; i < searchRadius * 1000000000; i++)
            {
            }

            var now = DateTime.Now.Minute % 2 == 0;
            var forceMutexLock = DateTime.Now.Minute % 2 == 0;
            if ("eu-north".Equals(region) && forceMutexLock)
            {
                MutexLock(searchRadius);
            }
        });
    }

    private static void MutexLock(long searchRadius)
    {
        for (long i = 0; i < 30 * searchRadius * 1000000000; i++)
        {
        }
    }
}