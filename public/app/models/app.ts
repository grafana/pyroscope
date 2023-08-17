import { parse, brandQuery, Query } from '@pyroscope/models/query';
import { SpyNameSchema, UnitsSchema } from '@pyroscope/legacy/models';
import { z } from 'zod';

export const PyroscopeAppLabel = 'pyroscope_app';
export const ServiceNameLabel = 'service_name';

const AppWithPyroscopeAppIndex = z.object({
  __profile_type__: z.string(),
  pyroscope_app: z.string(),
  // Fake a discriminated union
  __name_id__: z.enum([PyroscopeAppLabel]).default(PyroscopeAppLabel),
  name: z.string().optional().default(''),
});

const AppWithServiceNameIndex = z.object({
  __profile_type__: z.string(),
  service_name: z.string(),
  // Fake a discriminated union
  __name_id__: z.enum([ServiceNameLabel]).default(ServiceNameLabel),
  name: z.string().optional().default(''),
});

// Backwards compatibility,
// even though https://github.com/grafana/phlare/pull/710 is merged
// we can't guarantee backend is deployed to support that
export const BasicAppSchema = AppWithPyroscopeAppIndex.or(
  AppWithServiceNameIndex
).transform(enhanceWithName);

const ExtraFields = z.object({
  __type__: z.string(),
  __name__: z.string(),
});

export const AppSchema = AppWithPyroscopeAppIndex.merge(ExtraFields)
  .or(AppWithServiceNameIndex.merge(ExtraFields))
  .transform(enhanceWithName);

// Always populate the 'field' name, to make it easier for components that
// only need to display a name
function enhanceWithName<
  T extends
    | { __name_id__: 'pyroscope_app'; pyroscope_app: string; name: string }
    | {
        __name_id__: 'service_name';
        service_name: string;
        name: string;
      }
>(a: T) {
  if (a.__name_id__ === 'pyroscope_app') {
    a.name = a.pyroscope_app;
  }
  if (a.__name_id__ === 'service_name') {
    a.name = a.service_name;
  }
  return a;
}

export type App = z.infer<typeof AppSchema>;

export type BasicApp = z.infer<typeof BasicAppSchema>;

// Given a query returns an App
export function appFromQuery(
  query: Query
): z.infer<typeof BasicAppSchema> | undefined {
  const parsed = parse(query);

  if (!parsed) {
    return undefined;
  }

  const app = {
    __profile_type__: parsed?.profileId,
    ...parsed?.tags,
  };

  const parsedApp = BasicAppSchema.safeParse(app);
  if (!parsedApp.success) {
    return undefined;
  }

  return parsedApp.data;
}

export function appToQuery(app: z.infer<typeof BasicAppSchema>): Query {
  // Useless check just to satisfy type checking
  if (app.__name_id__ === 'pyroscope_app') {
    return brandQuery(
      `${app.__profile_type__}{${app.__name_id__}="${app[app.__name_id__]}"}`
    );
  }

  return brandQuery(
    `${app.__profile_type__}{${app.__name_id__}="${app[app.__name_id__]}"}`
  );
}

// TODO old App type
//
// export type App = z.infer<typeof appModel>;

export const appModel = z.object({
  name: z.string(),
  spyName: SpyNameSchema,
  units: UnitsSchema,
});

export const appsModel = z.array(appModel);
