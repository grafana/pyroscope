package org.example.rideshare;

import io.opentelemetry.api.trace.SpanContext;
import io.opentelemetry.context.Context;
import io.opentelemetry.sdk.trace.ReadWriteSpan;
import io.opentelemetry.sdk.trace.ReadableSpan;
import io.opentelemetry.sdk.trace.SpanProcessor;

public class RootSpanPrinterProcessor implements SpanProcessor {

    @Override
    public boolean isStartRequired() {
        return true;
    }

    @Override
    public boolean isEndRequired() {
        return false;
    }

    @Override
    public void onStart(Context parentContext, ReadWriteSpan span) {
        if (isRootSpan(span)) {
            System.out.println("root span id: " + span.getSpanContext().getSpanId());
        }
    }

    @Override
    public void onEnd(ReadableSpan span) {
    }

    private static boolean isRootSpan(ReadableSpan span) {
        SpanContext parent = span.getParentSpanContext();
        return parent == SpanContext.getInvalid() || parent.isRemote();
    }
}
