import React from 'react';
import { Maybe } from 'true-myth';
import type { ExploreTooltipProps } from './ExploreTooltip';

// TODO(eh-am): what are these units?
const THRESHOLD = 10000;

// TODO: fix types
export default function Annotations(
  props: ExploreTooltipProps & {
    annotations: { timestamp: number; content: string }[];
  }
) {
  if (!props.annotations?.length) {
    return null;
  }
  return getClosestTimestamp(props.values)
    .andThen((closest) => getClosestAnnotation(props.annotations, closest))
    .map((annotation) => (
      <AnnotationComponent
        timestamp={annotation.timestamp}
        content={annotation.content}
      />
    ))
    .unwrapOr(null);
}

function AnnotationComponent({
  timestamp,
  content,
}: {
  timestamp: number;
  content: string;
}) {
  return (
    <div>
      <div>timestamp: {timestamp}</div>
      <div>content: {content}</div>
    </div>
  );
}

function getClosestTimestamp(
  values: ExploreTooltipProps['values']
): Maybe<number> {
  if (!values) {
    return Maybe.nothing();
  }
  if (!values[0]) {
    return Maybe.nothing();
  }
  if (!values[0].closest) {
    return Maybe.nothing();
  }
  if (!values[0].closest[0]) {
    return Maybe.nothing();
  }
  return Maybe.of(values[0].closest[0]);
}

function getClosestAnnotation(
  annotations: { timestamp: number; content: string }[],
  timestamp: number
) {
  const f = annotations.find(
    (a) => Math.abs(a.timestamp - timestamp) < THRESHOLD
  );

  return Maybe.of(f);
}
