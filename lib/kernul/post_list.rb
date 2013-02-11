module Kernul
  class PostList
    def initialize(paths)
      @posts = paths.map { |p| Post.new(p) }.sort
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
