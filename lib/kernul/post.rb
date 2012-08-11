require "kernul/markdown"
require "date"

module Kernul
  class Post
    HEADER_REGEX = /^(.+) \/ (.+ \d{1,2}, \d{4})\n^=+\s*\n*/m

    attr_reader :permalink

    attr_reader :title

    attr_reader :date

    attr_reader :body

    def self.from_file(permalink, path)
      contents = File.read(path)

      if contents =~ HEADER_REGEX
        title = $1
        date = $2
        body = $'
        new(permalink, title, date, body)
      else
        raise ArgumentError, "Post has invalid format"
      end
    end

    def initialize(permalink, title, date, body)
      @permalink = permalink
      @title = title
      @date = DateTime.strptime(date, "%b %d, %Y")
      @body = body
    end

    def rendered_body
      MARKDOWN.render(body)
    end
  end
end
