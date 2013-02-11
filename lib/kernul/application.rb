module Kernul
  class Application < Sinatra::Application
    POST_LIST = PostList.new(Dir["posts/**/*.md"])

    use Rack::Static, :urls => ["/images", "/fonts", "/files"], :root => "assets"
    set :views, :default => "assets/templates", :scss => "assets/stylesheets"

    helpers do
      def find_template(views, name, engine, &block)
        _, folder = views.detect { |k,v| engine == Tilt[k] }
        folder ||= views[:default]
        super(folder, name, engine, &block)
      end
    end

    get "/" do
      posts = POST_LIST.latest(3)
      slim :index, :locals => { :posts => posts }
    end

    get "/post/:permalink" do
      post = POST_LIST.by_permalink(params[:permalink])
      halt 404 unless post
      slim :post, :locals => { :post => post }
    end

    get "/archive" do
      posts = POST_LIST.all
      slim :archive, :locals => { :posts => posts }
    end

    get "/feed" do
      posts = POST_LIST.latest(5)

      builder do |xml|
        xml.instruct! :xml, :version => "1.0", :encoding => "utf-8"
        xml.feed :xmlns => "http://www.w3.org/2005/Atom" do
          xml.title "Kernul"
          xml.link :href => "http://kernul.com", :rel => "self"
          xml.id "http://kernul.com"
          xml.updated posts.first.date.rfc3339

          posts.each do |post|
            xml.entry do
              xml.title post.title
              xml.link :rel => "alternate", :type => "text/html", :href => "http://kernul.com/post/#{post.permalink}"
              xml.id "http://kernul.com/post/#{post.permalink}"
              xml.updated post.date.rfc3339

              xml.content :type => "text/html" do
                xml.text! post.rendered_body
              end

              xml.author do
                xml.name "Alexander Kern"
                xml.email "alex@kernul.com"
              end
            end
          end
        end
      end
    end

    get "/stylesheets/application.css" do
      scss :application
    end
  end
end
