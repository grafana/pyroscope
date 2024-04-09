using System;
using Microsoft.Extensions.Logging;
using OpenTelemetry;

namespace Example;

class PyroscopeSpanProcessor : BaseProcessor<System.Diagnostics.Activity> {
  private readonly ILogger _logger;
  
  public PyroscopeSpanProcessor(ILoggerFactory loggerFactory)
  {
    _logger = loggerFactory.CreateLogger("main");
  }

  public override void OnStart(System.Diagnostics.Activity data) {
        _logger.LogInformation("OnStart()");
        if (data.RootId == null) {
            return;
        }
        _logger.LogInformation("PyroscopeSpanProcessor: Current thread ID: {0}", Environment.CurrentManagedThreadId);
        _logger.LogInformation("Setting dynamic tag {span_id}", data.SpanId.ToString());
        Pyroscope.Profiler.Instance.SetDynamicTag("profile_id", data.SpanId.ToString());
        Pyroscope.Profiler.Instance.SetDynamicTag("span_id", data.SpanId.ToString());
    }

    public override void OnEnd(System.Diagnostics.Activity data) {
        _logger.LogInformation("OnEnd()");
        if (data.RootId == null) {
            return;
        }
        _logger.LogInformation("PyroscopeSpanProcessor: Current thread ID: {0}", Environment.CurrentManagedThreadId);
        _logger.LogInformation("Setting dynamic tag {span_id}", data.SpanId.ToString());
        Pyroscope.Profiler.Instance.SetDynamicTag("profile_id", data.SpanId.ToString());
        Pyroscope.Profiler.Instance.SetDynamicTag("span_id", data.SpanId.ToString());
        data.AddTag("pyroscope.profile.id", data.SpanId.ToString());
    }
}
