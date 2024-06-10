import { allPosts } from "@/.contentlayer/generated";
import Link from "next/link";
import { Metadata } from "next";
import { metadata as layoutMetadata } from "@/app/layout";

export const metadata: Metadata = {
  ...layoutMetadata,
  title: "blog • kern.io",
  description: "Writings by Alex Kern",
  openGraph: {
    ...layoutMetadata.openGraph,
    title: "blog • kern.io",
    description: "Writings by Alex Kern",
    url: "https://kern.io/p",
  },
  twitter: {
    ...layoutMetadata.twitter,
    title: "blog • kern.io",
    description: "Writings by Alex Kern",
  },
};

export default function Home() {
  return (
    <div className="prose dark:prose-invert">
      {allPosts.map((post) => (
        <article key={post._id}>
          <Link href={post.slug}>
            <h2>{post.title}</h2>
          </Link>
          {post.description && <p>{post.description}</p>}
        </article>
      ))}
    </div>
  );
}
