import type { Profile, Groups } from '@pyroscope/legacy/models';
import type { Timeline } from '@pyroscope/models/timeline';
import type { Annotation } from '@pyroscope/models/annotation';
import type { App } from '@pyroscope/models/app';

type NewAnnotationState =
  | {
      type: 'pristine';
    }
  | { type: 'saving' };

type SingleView =
  | { type: 'pristine'; profile?: Profile }
  | { type: 'loading'; profile?: Profile }
  | {
      type: 'loaded';
      timeline: Timeline;
      profile: Profile;
      annotations: Annotation[];
    }
  | {
      type: 'reloading';
      timeline: Timeline;
      profile: Profile;
      annotations: Annotation[];
    };

type TagExplorerView = GroupByType &
  GroupsLoadingType &
  ActiveProfileType & {
    annotations: Annotation[];
  };

type GroupByType = {
  groupByTag: string;
  groupByTagValue: string;
};

type GroupsLoadingType =
  | {
      groupsLoadingType: 'pristine';
      groups: Groups;
    }
  | {
      groupsLoadingType: 'loading';
      groups: Groups;
    }
  | {
      groupsLoadingType: 'loaded';
      groups: Groups;
    }
  | {
      groupsLoadingType: 'reloading';
      groups: Groups;
    };

type ActiveProfileType =
  | {
      activeTagProfileLoadingType: 'pristine';
    }
  | {
      activeTagProfileLoadingType: 'loading';
    }
  | {
      activeTagProfileLoadingType: 'loaded';
      activeTagProfile: Profile;
    }
  | {
      activeTagProfileLoadingType: 'reloading';
      activeTagProfile: Profile;
    };

type ComparisonView = {
  left:
    | { type: 'pristine'; profile?: Profile }
    | { type: 'loading'; profile?: Profile }
    | { type: 'loaded'; profile: Profile }
    | { type: 'reloading'; profile: Profile };

  right:
    | { type: 'pristine'; profile?: Profile }
    | { type: 'loading'; profile?: Profile }
    | { type: 'loaded'; profile: Profile }
    | { type: 'reloading'; profile: Profile };

  comparisonMode: {
    active: boolean;
    period: {
      label: string;
      ms: number;
    };
  };
};

export type DiffView =
  | { type: 'pristine'; profile?: Profile }
  | { type: 'loading'; profile?: Profile }
  | { type: 'loaded'; profile: Profile }
  | { type: 'reloading'; profile: Profile };

type TimelineState =
  | { type: 'pristine'; timeline: Timeline }
  | { type: 'loading'; timeline: Timeline }
  | { type: 'reloading'; timeline: Timeline }
  | { type: 'loaded'; timeline: Timeline; annotations: Annotation[] };

type TagsData =
  | { type: 'pristine' }
  | { type: 'loading' }
  | { type: 'failed' }
  | { type: 'loaded'; values: string[] };

// Tags really refer to each application
// Should we nest them to an application?
export type TagsState =
  | { type: 'pristine'; tags: Record<string, TagsData> }
  | { type: 'loading'; tags: Record<string, TagsData> }
  | {
      type: 'loaded';
      tags: Record<string, TagsData>;
      from: number;
      until: number;
    }
  | { type: 'failed'; tags: Record<string, TagsData> };

// TODO
type appName = string;
type Tags = Record<appName, TagsState>;

export interface ContinuousState {
  from: string;
  until: string;
  leftFrom: string;
  leftUntil: string;
  rightFrom: string;
  rightUntil: string;
  query: string;
  leftQuery?: string;
  rightQuery?: string;
  maxNodes: string;
  aggregation: string;
  refreshToken?: string;

  singleView: SingleView;
  diffView: DiffView;
  comparisonView: ComparisonView;
  tagExplorerView: TagExplorerView;
  newAnnotation: NewAnnotationState;
  tags: Tags;

  apps:
    | { type: 'loaded'; data: App[] }
    | { type: 'reloading'; data: App[] }
    | { type: 'failed'; data: App[] };

  // Since both comparison and diff use the same timeline
  // Makes sense storing them separately
  leftTimeline: TimelineState;
  rightTimeline: TimelineState;
}
