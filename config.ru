require 'bundler'
Bundler.setup

require 'toto'

# Rack config
use Rack::Static, :urls => ['/css', '/js', '/images', '/fonts', '/favicon.ico'], :root => 'public'
use Rack::CommonLogger

if ENV['RACK_ENV'] == 'development'
  use Rack::ShowExceptions
end

class Toto::Site::Context
  attr_reader :path
end

# Toto config
toto = Toto::Server.new do
  set :author, 'Alexander Kern'
  set :title, 'Kernpedia'
  set :ext, 'txt'
  set :disqus, 'kernpedia'
end

run toto