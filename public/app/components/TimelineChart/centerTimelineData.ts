import type { Timeline } from '@pyroscope/models/timeline';

export interface TimelineData {
  data?: Timeline;
  color?: string;
}

function decodeTimelineData(timeline: Timeline) {
  if (!timeline) {
    return [];
  }
  let time = timeline.startTime;
  return timeline.samples.map((x) => {
    const res = [time * 1000, x];
    time += timeline.durationDelta;
    return res;
  });
}

// Since profiling data is chuked by 10 seconds slices
// it's more user friendly to point a `center` of a data chunk
// as a bar rather than starting point, so we add 5 seconds to each chunk to 'center' it
export function centerTimelineData(timelineData: TimelineData) {
  return timelineData.data
    ? decodeTimelineData(timelineData.data).map((x) => [
        x[0] + 5000,
        x[1] === 0 ? 0 : x[1] - 1,
      ])
    : [[]];
}
