$LOAD_PATH.unshift(File.expand_path("../lib", __FILE__))
require "bundler/setup"
require "rack/pygments"
require "kernul"

use Rack::Pygments

run Kernul::Application
