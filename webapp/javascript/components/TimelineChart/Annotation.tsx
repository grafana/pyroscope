import React from 'react';
import { Maybe } from 'true-myth';
import { format } from 'date-fns';
import styles from './Annotation.module.scss';

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
  // TODO: these don't account for timezone
  return (
    <section>
      <header className={styles.header}>
        {format(timestamp, 'yyyy-MM-dd hh:mm aa')}
      </header>
      <div>{content}</div>
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

// TODO(eh-am): threshold does not account for different time ranges
// we need to scale based on resolution (1 hour, 3 hours etc)
function getClosestAnnotation(
  annotations: { timestamp: number; content: string }[],
  timestamp: number
) {
  // Create a score based on how distant it is from the timestamp
  // Then get the first value (the closest to the timestamp)
  const f = annotations
    .map((a) => ({
      ...a,
      score: Math.abs(a.timestamp - timestamp),
    }))
    .filter((a) => a.score < THRESHOLD)
    .sort((a, b) => a.score - b.score);

  return Maybe.of(f[0]);
}
