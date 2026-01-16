// Wrapper to fix CommonJS/ESM interop for react-custom-scrollbars-2
const scrollbars = require('react-custom-scrollbars-2/lib/Scrollbars');
export default scrollbars.default || scrollbars;
export const Scrollbars = scrollbars.default || scrollbars;
