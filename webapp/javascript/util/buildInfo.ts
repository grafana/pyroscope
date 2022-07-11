// eslint-disable-next-line import/no-relative-packages
import { version as jsVersion } from '../../../package.json';

export interface BuildInfo {
  goos: string;
  goarch: string;
  goVersion: string;
  version: string;
  time: string;
  gitSHA: string;
  gitDirty: string;
  useEmbeddedAssets: string;
  jsVersion: string;
}

export const buildInfo = function (): BuildInfo {
  // TODO: it may be possible that these fields are not populated
  const win = window as unknown as { buildInfo: BuildInfo };

  return {
    jsVersion,
    goos: win.buildInfo?.goos,
    goarch: win.buildInfo?.goarch,
    goVersion: win.buildInfo?.goVersion,
    version: win.buildInfo?.version,
    time: win.buildInfo?.time,
    gitSHA: win.buildInfo?.gitSHA,
    gitDirty: win.buildInfo?.gitDirty,
    useEmbeddedAssets: win.buildInfo?.useEmbeddedAssets,
  };
};

interface LatestVersionInfo {
  latest_version: string;
}

export const latestVersionInfo = function (): LatestVersionInfo {
  // TODO: it may be possible that these fields are not populated
  const win = window as unknown as { latestVersionInfo: LatestVersionInfo };

  return {
    latest_version: win.latestVersionInfo.latest_version,
  };
};
