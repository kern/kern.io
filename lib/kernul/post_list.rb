require "kernul/post"

module Kernul
  class PostList
    def initialize(pattern = "assets/posts/**/*.md")
      @posts = Dir[pattern].reverse.map do |p|
        basename = File.basename(p, ".md")
        if basename =~ /^\d{4}-\d{2}-\d{2}-(.+)$/
          permalink = $1
          Post.from_file(permalink, p)
        else
          raise ArgumentError, "Invalid post basename '#{basename}'."
        end
      end
    end

    def latest(n)
      @posts.take(n)
    end

    def all
      @posts
    end

    def by_permalink(permalink)
      @posts.find { |p| p.permalink == permalink }
    end
  end
end
