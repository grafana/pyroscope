const { merge } = require("webpack-merge");

const path = require("path");
const prod = require("./webpack.prod.js");

module.exports = merge(prod, {
  entry: {
    flamegraphComponent:
      "./webapp/javascript/components/FlameGraph/FlameGraphComponent/index.jsx",
  },

  output: {
    publicPath: "",
    path: path.resolve(__dirname, "../../webapp/public/assets"),
    filename: "[name].js",
    clean: true,

    library: "pyroscope",
    libraryTarget: "umd",
    umdNamedDefine: true,
  },
});
