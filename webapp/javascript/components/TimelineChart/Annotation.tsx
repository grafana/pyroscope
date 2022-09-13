import React from 'react';
import { Maybe } from 'true-myth';

// TODO(eh-am): what are these units?
export const THRESHOLD = 10000;

interface AnnotationTooltipBodyProps {
  /** list of flotjs datapoints being hovered. we only use the first one */
  values?: { closest: number[] }[];
  /** list of annotations */
  annotations: { timestamp: number; content: string }[];
}

export default function Annotations(props: AnnotationTooltipBodyProps) {
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
    <section>
      <div>timestamp: {timestamp}</div>
      <div>content: {content}</div>
    </section>
  );
}

function getClosestTimestamp(values?: { closest: number[] }[]): Maybe<number> {
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
