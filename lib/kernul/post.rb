module Kernul
  class Post
    HEADER_REGEX = /^(.+) \/ (.+) \/ (.+)$/

    attr_reader :title
    attr_reader :date
    attr_reader :permalink
    attr_reader :body

    def initialize(path)
      contents = File.read(path)

      md = HEADER_REGEX.match(contents) or raise InvalidPostHeader
      @title = md[1]
      @date = DateTime.strptime(md[2], "%b %d, %Y")
      @permalink = md[3]
      @body = Markdown.new(md.post_match).to_html
    end

    def <=>(other)
      other.date <=> date
    end
  end
end
