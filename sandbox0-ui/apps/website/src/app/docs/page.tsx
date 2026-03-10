import { redirect } from "next/navigation";
import { defaultDocsPageSlug } from "@/app/docs/content";

export default function DocsIndexPage() {
  redirect(`/docs/latest/${defaultDocsPageSlug}`);
}
