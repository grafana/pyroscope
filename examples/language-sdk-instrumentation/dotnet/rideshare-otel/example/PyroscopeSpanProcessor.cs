using OpenTelemetry;

namespace Example;

class PyroscopeSpanProcessor : BaseProcessor<System.Diagnostics.Activity> {

    public override void OnStart(System.Diagnostics.Activity data) {
        if (data.RootId == null) {
            return;
        }
        Pyroscope.Profiler.Instance.SetDynamicTag("profile_id", data.RootId);
        Pyroscope.Profiler.Instance.SetDynamicTag("span_name", data.OperationName);
        data.AddTag("pyroscope.profile.id", data.RootId);
    }

    public override void OnEnd(System.Diagnostics.Activity data) {

    }
}
