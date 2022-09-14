import React from 'react';
import { Maybe } from 'true-myth';
import { format } from 'date-fns';
import styles from './Annotation.module.scss';

// TODO(eh-am): what are these units?
export const THRESHOLD = 3;

interface AnnotationTooltipBodyProps {
  /** list of flotjs datapoints being hovered. we only use the first one */
  values?: { closest: number[] }[];
  /** list of annotations */
  annotations: { timestamp: number; content: string }[];

  /** given a timestamp, it returns the offset within the canvas */
  pointOffset: (coords: { x: number }) => { left: number };
}

export default function Annotations(props: AnnotationTooltipBodyProps) {
  if (!props.annotations?.length) {
    return null;
  }

  return getClosestTimestamp(props.values)
    .andThen((closest) =>
      getClosestAnnotation(props.annotations, closest, props.pointOffset)
    )
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

function getClosestAnnotation(
  annotations: { timestamp: number; content: string }[],
  timestamp: number,
  pointOffset: AnnotationTooltipBodyProps['pointOffset']
) {
  if (!annotations.length) {
    return Maybe.nothing<typeof annotations[number]>();
  }

  const timestampLeft = pointOffset({ x: timestamp }).left;

  // Create a score based on how distant it is from the timestamp
  // Then get the first value (the closest to the timestamp)
  const f = annotations
    .map((a) => ({
      ...a,
      score: Math.abs(pointOffset({ x: a.timestamp }).left - timestampLeft),
    }))
    .filter((a) => a.score < THRESHOLD)
    .sort((a, b) => a.score - b.score);

  return Maybe.of(f[0]);
}
