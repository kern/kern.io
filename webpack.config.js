const FaviconsWebpackPlugin = require("favicons-webpack-plugin");
const CopyWebpackPlugin = require("copy-webpack-plugin");
const ExtractTextWebpackPlugin = require("extract-text-webpack-plugin");
const IgnoreAssetsWebpackPlugin = require("ignore-assets-webpack-plugin");
const StyleExtHTMLWebpackPlugin = require("style-ext-html-webpack-plugin");
const HTMLWebpackPlugin = require("html-webpack-plugin");
const nib = require("nib");
const path = require("path");
const webpack = require("webpack");

const INDEX_PATH = path.resolve(__dirname, "index.html");
const STYLES_PATH = path.resolve(__dirname, "index.styl");
const FAVICON_PATH = path.resolve(__dirname, "favicon.png");

module.exports = {
  target: "web",
  entry: [INDEX_PATH, STYLES_PATH],
  output: {
    filename: "bundle.js",
    publicPath: "/"
  },
  module: {
    rules: [
      {
        test: /\.(html)$/,
        loader: "html-loader",
        options: {
          minimize: true
        }
      },
      {
        test: /\.(css|styl)$/,
        loader: ExtractTextWebpackPlugin.extract({
          fallback: { loader: "style-loader" },
          use: [
            "css-loader",
            {
              loader: "stylus-loader",
              options: {
                use: [nib()]
              }
            }
          ]
        })
      },
      {
        test: /\.(eot|svg|ttf|woff|woff2)$/,
        loader: "file-loader?name=fonts/[hash].[ext]"
      },
      {
        test: /\.(png)$/,
        loader: "url-loader?limit=100000"
      }
    ]
  },
  plugins: [
    new webpack.NoEmitOnErrorsPlugin(),
    new HTMLWebpackPlugin({
      filename: "index.html",
      template: INDEX_PATH,
      chunks: ["index.css"]
    }),
    new ExtractTextWebpackPlugin("index.css"),
    new StyleExtHTMLWebpackPlugin({ minify: true }),
    new CopyWebpackPlugin([{ from: "files", to: "files" }]),
    new FaviconsWebpackPlugin({ logo: FAVICON_PATH, prefix: "icons/[hash]/" }),
    new IgnoreAssetsWebpackPlugin({
      ignore: "bundle.js"
    })
  ]
};
