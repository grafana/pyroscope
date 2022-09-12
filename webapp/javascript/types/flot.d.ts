import 'flot';

// @types/flot only exposes plotOptions
// but flot in fact exposes more parameters to us
// https://github.com/flot/flot/blob/370cf6ee85de0e0fcae5bf084e0986cda343e75b/source/jquery.flot.js#L361
type plotInitPluginParams = jquery.flot.plot & jquery.flot.plotOptions;
declare global {
  declare namespace jquery.flot {
    interface plugin {
      init(plot: plotInitPluginParams): void;
    }
  }
}
