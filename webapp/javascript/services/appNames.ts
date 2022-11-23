import { Result } from '@webapp/util/fp';
import { AppNames } from '@webapp/models/appNames';
import { fetchApps } from '@webapp/services/apps';
import type { ZodError } from 'zod';
import type { RequestError } from './base';

// Due to circunstances, older versions of pyroscope accepted apps with empty names
// TODO: maybe also check for illegal characters and whatnot?
function isValidAppName(appName: string) {
  return appName.trim().length > 0;
}

export async function fetchAppNames(): Promise<
  Result<AppNames, RequestError | ZodError>
> {
  return (await fetchApps())
    .map((apps) => apps.map((a) => a.name))
    .map((a) => a.filter(isValidAppName));
}
