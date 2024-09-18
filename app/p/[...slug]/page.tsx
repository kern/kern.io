import { notFound } from "next/navigation";
import { allPosts } from "contentlayer/generated";

import { metadata as layoutMetadata } from "@/app/layout";
import { Metadata } from "next";
import { Mdx } from "@/components/mdx-components";
import Link from "next/link";

interface PostProps {
  params: {
    slug: string[];
  };
}

async function getPostFromParams(params: PostProps["params"]) {
  const slug = params?.slug?.join("/");
  const sortedPosts = allPosts.sort(
    (a, b) => new Date(b.date).getTime() - new Date(a.date).getTime(),
  );

  const postIndex = sortedPosts.findIndex((post) => post.slugAsParams === slug);
  const post = allPosts[postIndex];

  if (!post) {
    return null;
  }

  const nextPost = sortedPosts[postIndex + 1];
  const prevPost = sortedPosts[postIndex - 1];

  return {
    ...post,
    next: nextPost ? nextPost : null,
    prev: prevPost ? prevPost : null,
  };
}

export async function generateMetadata({
  params,
}: PostProps): Promise<Metadata> {
  const post = await getPostFromParams(params);

  if (!post) {
    return {};
  }

  const title = post.title + " • kern.io";
  const description = post.description;

  return {
    ...layoutMetadata,
    title,
    description,
    openGraph: {
      ...layoutMetadata.openGraph,
      title,
      description,
      url: post.slug,
    },
    twitter: {
      ...layoutMetadata.twitter,
      title,
      description,
    },
  };
}

export async function generateStaticParams(): Promise<PostProps["params"][]> {
  return allPosts.map((post) => ({
    slug: post.slugAsParams.split("/"),
  }));
}

export default async function PostPage({ params }: PostProps) {
  const post = await getPostFromParams(params);

  if (!post) {
    notFound();
  }

  return (
    <article className="py-6 prose dark:prose-invert">
      <h1 className="mb-2">{post.title}</h1>
      {post.description && (
        <p className="text-xl mt-0 text-slate-700 dark:text-slate-200">
          {post.description}
        </p>
      )}
      <hr className="my-4" />
      <Mdx code={post.body.code} />
      <div className="flex justify-between w-full gap-4 mt-16">
        <PostNavigation post={post.prev} direction="prev" />
        <PostNavigation post={post.next} direction="next" />
      </div>
    </article>
  );
}

function PostNavigation({
  post,
  direction,
}: {
  post: any;
  direction: "prev" | "next";
}) {
  if (!post) {
    return <div className="w-[50%]" />;
  }

  const label = direction === "prev" ? "« Newer" : "Older »";
  const textAlign = direction === "prev" ? "text-left" : "text-right";

  return (
    <Link
      href={post.slug}
      className={`block no-underline w-[50%] self-stretch ${textAlign}`}
    >
      <div className="h-full rounded bg-white dark:bg-stone-800 border border-stone-200 dark:border-stone-700 shadow-sm hover:shadow-md active:shadow-inset transition-shadow duration-300 px-4 py-4">
        <p className="text-sm text-stone-600 m-0">{label}</p>
        <h3 className="my-0 text-lg font-semibold leading-tight">
          {post.title}
        </h3>
      </div>
    </Link>
  );
}
