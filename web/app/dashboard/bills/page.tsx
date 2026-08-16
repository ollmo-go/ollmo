"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";

// The standalone bills page was folded into the analytics page as its
// "用量与费用" tab. Keep the old URL working via a redirect.
export default function BillsRedirect() {
  const router = useRouter();
  useEffect(() => {
    router.replace("/dashboard/analytics?tab=bills");
  }, [router]);
  return null;
}
