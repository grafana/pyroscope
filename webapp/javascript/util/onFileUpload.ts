import { deltaDiffWrapper } from '../util/flamebearer';

export default function onFileUpload(data, setFlamebearer) {
  if (!data) {
    setFlamebearer(null);
    return;
  }

  const { flamebearer } = data;

  const calculatedLevels = deltaDiffWrapper(
    flamebearer.format,
    flamebearer.levels
  );

  flamebearer.levels = calculatedLevels;
  setFlamebearer(flamebearer);
}
