import { Flamebearer, Profile } from '@pyroscope/legacy/models';
import { decodeFlamebearer } from './decode';

// normalize receives either a Profile or a Flamebearer
// then generates an usable 'Flamebearer', the expected format downstream
export function normalize(p: {
  profile?: Profile;
  flamebearer?: Flamebearer;
}): Flamebearer {
  if (p.profile && p.flamebearer) {
    console.warn(
      "'profile' and 'flamebearer' properties are mutually exclusive. Please use profile if possible."
    );
  }

  if (p.profile) {
    const copy = {
      ...p.profile,
      flamebearer: { ...p.profile.flamebearer },
    };

    // TODO: copy levels, since that's modified by decode
    copy.flamebearer.levels = JSON.parse(
      JSON.stringify(copy.flamebearer.levels)
    );
    decodeFlamebearer(copy);

    const p2 = {
      ...copy,
      ...copy.metadata,
      ...copy.flamebearer,

      // We won't need these fields again
      flamebearer: undefined,
      metadata: undefined,
    };

    delete p2.flamebearer;
    delete p2.metadata;
    return p2 as Flamebearer;
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
