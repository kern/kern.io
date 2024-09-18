import { allPosts } from "@/.contentlayer/generated";
import Link from "next/link";
import { Metadata } from "next";
import { metadata as layoutMetadata } from "@/app/layout";

export const metadata: Metadata = {
  ...layoutMetadata,
  title: "writing • kern.io",
  description: "Writings by Alex Kern",
  openGraph: {
    ...layoutMetadata.openGraph,
    title: "writing • kern.io",
    description: "Writings by Alex Kern",
    url: "https://kern.io/p",
  },
  twitter: {
    ...layoutMetadata.twitter,
    title: "writing • kern.io",
    description: "Writings by Alex Kern",
  },
};

export default function Home() {
  const sortedPosts = allPosts.sort(
    (a, b) => new Date(b.date).getTime() - new Date(a.date).getTime(),
  );
  return (
    <div className="flex flex-wrap justify-center gap-4">
      {sortedPosts.map((post) => (
        <div
          key={post._id}
          className="w-full rounded bg-white dark:bg-stone-800 border border-stone-200 dark:border-stone-700 shadow-sm hover:shadow-lg active:shadow-inset transition-shadow duration-300"
        >
          <Link href={post.slug}>
            <div className="px-6 py-4 flex flex-col justify-between">
              <div>
                <h2 className="font-bold text-xl mb-2">{post.title}</h2>
                {post.description && (
                  <p className="text-stone-700 dark:text-stone-200 text-base">
                    {post.description}
                  </p>
                )}
              </div>
            </div>
          </Link>
        </div>
      ))}
    </div>
  );
}
