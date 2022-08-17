export type PlotType = {
  getPlotOffset: () => ShamefulAny;
  getOptions: () => ShamefulAny;
  getAxes: () => ShamefulAny[];
  getXAxes: () => ShamefulAny[];
  getYAxes: () => ShamefulAny[];
  getPlaceholder: () => {
    trigger: (arg0: string, arg1: ShamefulAny[]) => void;
    offset: () => {
      left: number;
      top: number;
    };
  };
  triggerRedrawOverlay: () => void;
  width: () => number;
  height: () => number;
  clearSelection: (preventEvent: boolean) => void;
  setSelection: (ranges: ShamefulAny, preventEvent: ShamefulAny) => void;
  getSelection: () => ShamefulAny | null;
  hooks: ShamefulAny;
};

export type CtxType = {
  save: () => void;
  translate: (arg0: ShamefulAny, arg1: ShamefulAny) => void;
  strokeStyle: ShamefulAny;
  lineWidth: number;
  lineJoin: ShamefulAny;
  fillStyle: ShamefulAny;
  fillRect: (arg0: number, arg1: number, arg2: number, arg3: number) => void;
  strokeRect: (arg0: number, arg1: number, arg2: number, arg3: number) => void;
  restore: () => void;
};

export type EventHolderType = {
  unbind: (
    arg0: string,
    arg1: { (e: ShamefulAny): void; (e: ShamefulAny): void }
  ) => void;
  mousemove: (arg0: (e: EventType) => void) => void;
  mousedown: (arg0: (e: EventType) => void) => void;
  mouseleave: (arg0: (e: EventType) => void) => void;
};

export type EventType = { pageX: number; pageY: number; which?: number };
