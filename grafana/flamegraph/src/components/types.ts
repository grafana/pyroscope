export type TooltipData = {
  name: string,
  percentTitle: string,
  percentValue: number,
  unitTitle: string,
  unitValue: string,
  samples: string
}

export enum SampleUnit {
  Bytes = 'bytes',
  None = 'none',
  Nanoseconds = 'ns'
}
