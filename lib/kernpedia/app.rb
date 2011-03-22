require 'sinatra/base'
require 'erb'

module Kernpedia
  class App < Sinatra::Base
    include Sinatra::Partials
    set :public, './public'
    
    get '/' do
      erb :welcome
    end
  end
end