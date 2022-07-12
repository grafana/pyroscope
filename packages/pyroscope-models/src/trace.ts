import { z } from 'zod';

const ReferencesSchema = z.object({
  refType: z.string(),
  traceID: z.string(),
  spanID: z.string(),
});

const TagsSchema = z.object({
  key: z.string(),
  type: z.string(),
  value: z.union([z.boolean(), z.number(), z.string()]),
});

const TraceSpanSchema = z.object({
  traceID: z.string(),
  spanID: z.string(),
  flags: z.string(),
  operationName: z.string(),
  references: z.array(ReferencesSchema),
  startTime: z.number(),
  duration: z.number(),
  tags: z.array(TagsSchema),
  logs: z.object({
    timestamp: z.number(),
    fields: z.array(TagsSchema),
  }),
  processID: z.string(),
  warnings: z.any(),
});

const ProcessSchema = z.object({
  serviceName: z.string(),
  tags: z.array(TagsSchema),
});

const TraceSchema = z.object({
  traceID: z.string(),
  spans: z.array(TraceSpanSchema),
  processes: z.record(ProcessSchema),
  warnings: z.any(),
});

export type Trace = z.infer<typeof TraceSchema>;
export type TraceSpan = z.infer<typeof TraceSpanSchema>;
