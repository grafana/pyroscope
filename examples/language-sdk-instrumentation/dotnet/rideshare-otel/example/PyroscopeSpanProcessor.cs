using System;
using OpenTelemetry;

namespace Example;

class PyroscopeSpanProcessor : BaseProcessor<System.Diagnostics.Activity>
{

    private const string ProfileIdSpanTagKey = "pyroscope.profile.id";

    public override void OnStart(System.Diagnostics.Activity data)
    {
        if (!IsRootSpan(data)) {
            return;
        }
        var spanId = data.SpanId.ToString();

        try
        {
            ulong spanIdUlong = Convert.ToUInt64(spanId.ToUpper(), 16);
            Pyroscope.Profiler.Instance.SetProfileId(spanIdUlong);
        }
        catch (Exception ex)
        {
            Console.WriteLine($"Caught exception while setting profile id profiler: {ex.Message}");
        }

        data.AddTag(ProfileIdSpanTagKey, spanId);
    }

    private static bool IsRootSpan(System.Diagnostics.Activity data)
    {
        var parent = data.Parent;
        return parent == null || parent.HasRemoteParent;
    }
}
