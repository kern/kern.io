require "sinatra/base"
require "mustache/sinatra"
require "kernul/views/index"

module Kernul
  class Application < Sinatra::Application
    use Rack::Static, urls: ["/images", "/fonts"], root: "assets"
    set :views, scss: "assets/stylesheets", default: "assets"
    set :mustache, { views: "keystone/views", templates: "assets/templates", namespace: Kernul::Views }

    helpers do
      def find_template(views, name, engine, &block)
        _, folder = views.detect { |k,v| engine == Tilt[k] }
        folder ||= views[:default]
        super(folder, name, engine, &block)
      end
    end

    get "/" do
      mustache :index
    end

    get "/stylesheets/application.css" do
      scss :application, style: :expanded
    end
  end
end
