import { Flamebearer, Profile } from '@pyroscope/models/src';
import decode from './decode';

// normalize receives either a Profile or a Flamebearer
// then generates an usable 'Flamebearer', the expected format downstream
export function normalize(p: { profile?: Profile; flamebearer?: Flamebearer }) {
  if (p.profile && p.flamebearer) {
    console.warn(
      "'profile' and 'flamebearer' properties are mutually exclusive. Please use profile if possible."
    );
  }

  if (p.profile) {
    // TODO: copy levels, since that's modified by decode
    const copy = JSON.parse(JSON.stringify(p.profile));
    const profile = decode(copy);

    // TODO: clean this
    return {
      leftTicks: profile.leftTicks,
      rightTicks: profile.rightTicks,
      ...profile.flamebearer,
      ...profile.metadata,
    } as Flamebearer;
  }

  if (p.flamebearer) {
    return p.flamebearer;
  }

  // people may send us both values as undefined
  // but we still have to render something
  const noop: Flamebearer = {
    format: 'single',
    names: [],
    units: 'unknown',
    levels: [[]],
    spyName: 'unknown',
    numTicks: 0,
    sampleRate: 0,
    maxSelf: 0,
  };
  return noop;
}
