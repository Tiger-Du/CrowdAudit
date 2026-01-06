// // app/social/page.tsx
// import { getCommunityConversations } from "@/src/app/actions/social";
// import { VoteCard } from "@/components/evaluate-pair"; // Reuse your component

// export default async function SocialPage() {
//   const conversations = await getCommunityConversations();

//   return (
//     <div className="max-w-[1100px] mx-auto p-4">
//       <header className="mb-8 text-center">
//         <h1 className="text-2xl font-bold">Community Alignment</h1>
//         <p className="text-muted-foreground text-sm">
//           Review how others have evaluated model responses.
//         </p>
//       </header>

//       <div className="space-y-12">
//         {conversations.map((conv) => (
//           <section key={conv.conversation_id} className="border-b pb-8">
//             <div className="mb-4">
//               <span className="text-xs font-mono bg-muted px-2 py-1 rounded">
//                 ID: {conv.conversation_id} | Lang: {conv.assigned_lang}
//               </span>
//               <h3 className="mt-2 font-semibold">User Prompt:</h3>
//               <p className="text-sm text-muted-foreground whitespace-pre-wrap italic">
//                 "{conv.first_turn_prompt}"
//               </p>
//             </div>

//             <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
//               <VoteCard
//                 title="Response A"
//                 body={conv.first_turn_response_a}
//               />
//               <VoteCard
//                 title="Response B"
//                 body={conv.first_turn_response_b}
//               />
//             </div>

//             {conv.first_turn_feedback && (
//               <div className="mt-4 p-3 bg-blue-50 border border-blue-100 rounded-md">
//                 <p className="text-sm font-medium text-blue-900">
//                   Annotator Feedback:
//                 </p>
//                 <p className="text-sm text-blue-800 italic">
//                   {conv.first_turn_feedback}
//                 </p>
//               </div>
//             )}
//           </section>
//         ))}
//       </div>
//     </div>
//   );
// }
