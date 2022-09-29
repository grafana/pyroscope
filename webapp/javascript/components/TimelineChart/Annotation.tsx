import React from 'react';
import { Maybe } from 'true-myth';
import { format } from 'date-fns';
import { Annotation } from '@webapp/models/annotation';
import { getUTCdate, timezoneToOffset } from '@webapp/util/formatDate';
import styles from './Annotation.module.scss';

// TODO(eh-am): what are these units?
export const THRESHOLD = 3;

interface AnnotationTooltipBodyProps {
  /** list of annotations */
  annotations: { timestamp: number; content: string }[];

  /** given a timestamp, it returns the offset within the canvas */
  coordsToCanvasPos: jquery.flot.axis['p2c'];

  /* where in the canvas the mouse is */
  canvasX: number;

  timezone: 'browser' | 'utc';
}

export default function Annotations(props: AnnotationTooltipBodyProps) {
  if (!props.annotations?.length) {
    return null;
  }

  return getClosestAnnotation(
    props.annotations,
    props.coordsToCanvasPos,
    props.canvasX
  )
    .map((annotation: Annotation) => (
      <AnnotationComponent
        timezone={props.timezone}
        timestamp={annotation.timestamp}
        content={annotation.content}
      />
    ))
    .unwrapOr(null);
}

function AnnotationComponent({
  timestamp,
  content,
  timezone,
}: {
  timestamp: number;
  content: string;
  timezone: AnnotationTooltipBodyProps['timezone'];
}) {
  // TODO: these don't account for timezone
  return (
    <div className={styles.wrapper}>
      <header className={styles.header}>
        {format(
          getUTCdate(new Date(timestamp), timezoneToOffset(timezone)),
          'yyyy-MM-dd HH:mm'
        )}
      </header>
      <div className={styles.body}>{content}</div>
    </div>
  );
}

function getClosestAnnotation(
  annotations: { timestamp: number; content: string }[],
  coordsToCanvasPos: AnnotationTooltipBodyProps['coordsToCanvasPos'],
  canvasX: number
): Maybe<typeof annotations[number]> {
  if (!annotations.length) {
    return Maybe.nothing<typeof annotations[number]>();
  }

  // pointOffset requires a y position, even though we don't use it
  const dummyY = -1;

  // Create a score based on how distant it is from the timestamp
  // Then get the first value (the closest to the timestamp)
  const f = annotations
    .map((a) => ({
      ...a,
      score: Math.abs(
        coordsToCanvasPos({ x: a.timestamp, y: dummyY }).left - canvasX
      ),
    }))
    .filter((a) => a.score < THRESHOLD)
    .sort((a, b) => a.score - b.score);

  return Maybe.of(f[0]);
}
