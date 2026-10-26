using System;
using System.IO;
using System.IO.Compression;
using System.Text.Json;
using System.Collections.Generic;
using System.Security.Cryptography;
using System.Text.RegularExpressions;

// Public research evidence reader. No SQL, model, credential, or write interface.
public static class Issue76AuthorityForensicUnsealedV1 {
 const int Batch=4096, Line=8192, MaxSegments=16384;
 const long CompressedCap=536870912;
 static bool Member(string list,string value) { return Array.IndexOf(list.Split('|',StringSplitOptions.RemoveEmptyEntries),value)>=0; }
 static void Need(bool b,string code) { if(!b) throw new InvalidDataException("authority_stream:"+code); }
 static string S(JsonElement o,string k) { var v=o.GetProperty(k);Need(v.ValueKind==JsonValueKind.String,"string");return v.GetString(); }
 static ulong U(JsonElement o,string k) { ulong n;Need(o.GetProperty(k).TryGetUInt64(out n),"unsigned_integer");return n; }
 static long I(JsonElement o,string k) { long n;Need(o.GetProperty(k).TryGetInt64(out n),"integer");return n; }
 static bool B(JsonElement o,string k) {var v=o.GetProperty(k);Need(v.ValueKind==JsonValueKind.True||v.ValueKind==JsonValueKind.False,"boolean");return v.GetBoolean();}
 static void Keys(JsonElement o,string required,string optional="") {
  Need(o.ValueKind==JsonValueKind.Object,"object");var req=new HashSet<string>(required.Split(' ',StringSplitOptions.RemoveEmptyEntries));
  var allowed=new HashSet<string>(req);foreach(var k in optional.Split(' ',StringSplitOptions.RemoveEmptyEntries))allowed.Add(k);
  var seen=new HashSet<string>();foreach(var p in o.EnumerateObject())Need(allowed.Contains(p.Name)&&seen.Add(p.Name),"unknown_or_duplicate_field");
  foreach(var k in req)Need(seen.Contains(k),"missing_field");
 }
 static string Hash(Stream s) {return Convert.ToHexString(SHA256.HashData(s)).ToLowerInvariant();}
 static void Hex(string h) {Need(h!=null&&Regex.IsMatch(h,"\\A[0-9a-f]{64}\\z"),"hash_format");}
 public static string SafePath(string root,string rel) {
  Need(!String.IsNullOrWhiteSpace(rel)&&!Path.IsPathRooted(rel)&&!rel.Contains('\\')&&!rel.Contains(':'),"path");
  root=Path.GetFullPath(root);Need((File.GetAttributes(root)&FileAttributes.ReparsePoint)==0,"reparse");var p=root;
  foreach(var part in rel.Split('/')) {Need(part!=""&&part!="."&&part!="..","path");p=Path.Combine(p,part);Need((File.GetAttributes(p)&FileAttributes.ReparsePoint)==0,"reparse");}
  Need(File.Exists(p),"missing_file");return p;
 }
 sealed class State {
  public ulong Seq,Span,Errors;public int Peak;public Dictionary<ulong,string> Active=new Dictionary<ulong,string>();
  public void Event(JsonElement e) {
   Keys(e,"sequence span phase edge at_utc elapsed_us original_context_deadline_utc original_context_time_left_us query_family outcome context_cause pool","public_sql_template_sha256 sqlstate");
   Need(U(e,"sequence")==++Seq,"sequence");ulong span=U(e,"span");string phase=S(e,"phase"),edge=S(e,"edge"),family=S(e,"query_family"),outcome=S(e,"outcome");
   Need(phase=="query"||phase=="pool_acquire","phase");Need(Member("|pool_acquire|other_query|schedule_check|delegation_check|schedule_probe|delegation_metadata|owner_assignment|owner_mandate|owner_purpose_policy|owner_goal|lease|source_guard|restore_watermark|deletion_manifest|source_pins|budget|task_current|run_current|transaction_control|",family),"family");
   DateTimeOffset t;Need(DateTimeOffset.TryParse(S(e,"at_utc"),out t),"time");Need(I(e,"elapsed_us")>=0,"elapsed");
   var dl=e.GetProperty("original_context_deadline_utc");var left=e.GetProperty("original_context_time_left_us");
   Need((dl.ValueKind==JsonValueKind.Null)==(left.ValueKind==JsonValueKind.Null),"deadline_pair");
   if(dl.ValueKind!=JsonValueKind.Null){Need(dl.ValueKind==JsonValueKind.String&&DateTimeOffset.TryParse(dl.GetString(),out t),"deadline");I(e,"original_context_time_left_us");}
   const string errors="|deadline|cancelled|postgres_error|other_error|execution_authority_check_failed|execution_ownership_changed|execution_authority_observation_timeout|execution_lease_renewal_failed|memory_source_stale|memory_source_check_timeout|";
   string cause=S(e,"context_cause");Need(cause=="none"||Member(errors,cause),"context_cause");
   string hash="";JsonElement v;if(e.TryGetProperty("public_sql_template_sha256",out v)){hash=S(e,"public_sql_template_sha256");Hex(hash);}
   if(e.TryGetProperty("sqlstate",out v)){string code=S(e,"sqlstate");Need(Member("|40001|40P01|55P03|57014|08000|08001|08003|08004|08006|08007|08P01|53300|53400|53200|55000|57P01|57P02|57P03|25P02|25006|42P01|42703|42501|23505|23503|22023|22P02|",code),"sqlstate");Need(outcome=="postgres_error","sqlstate_outcome");}
   var pool=e.GetProperty("pool");Keys(pool,"available max_conns total_conns acquired_conns idle_conns");B(pool,"available");foreach(var k in new[]{"max_conns","total_conns","acquired_conns","idle_conns"})Need(I(pool,k)>=0,"pool_count");
   string binding=phase+"|"+family+"|"+hash;
   if(edge=="start") {Need(span==++Span&&outcome=="pending"&&I(e,"elapsed_us")==0,"start");Need(Active.Count<Batch,"active_bound");Active.Add(span,binding);Peak=Math.Max(Peak,Active.Count);}
   else {Need(edge=="end","edge");Need(outcome=="ok"||Member(errors,outcome),"outcome");string start;Need(Active.TryGetValue(span,out start)&&start==binding,"span_pair");Active.Remove(span);if(outcome!="ok")Errors++;}
  }
 }
 public static string Read(string directory,string expectedIdentityJson,string expectedHeaderSha256) {
  try{return ReadCore(directory,expectedIdentityJson,expectedHeaderSha256);}
  catch(InvalidDataException){throw;}catch{throw new InvalidDataException("authority_stream:malformed_or_io");}
 }
 static string ReadCore(string directory,string expectedIdentityJson,string expectedHeaderSha256) {
  Hex(expectedHeaderSha256);string header=SafePath(directory,"authority-trace.json");Need(new FileInfo(header).Length<=16777216,"header_bound");
  using(var f=File.OpenRead(header))Need(Hash(f)==expectedHeaderSha256,"header_hash");
  using var doc=JsonDocument.Parse(File.ReadAllBytes(header));var h=doc.RootElement;
  Keys(h,"version sealed event_count last_sequence maximum_buffered_events maximum_segments maximum_compressed_bytes total_compressed_bytes identity segments tail open_spans dropped_events operation_failures observer_failed");
  Need(S(h,"version")=="behavior76-authority-pg-trace-stream-v2"&&!B(h,"sealed"),"version_or_unsealed");
  Need(!B(h,"observer_failed")&&U(h,"open_spans")==0&&U(h,"dropped_events")==0,"observer_failure");
  Need(U(h,"maximum_buffered_events")==Batch&&U(h,"maximum_segments")==MaxSegments&&U(h,"maximum_compressed_bytes")==CompressedCap,"limits");
  using var idDoc=JsonDocument.Parse(expectedIdentityJson);var id=h.GetProperty("identity");Keys(id,"window arm schema producer protocol_sha256");Keys(idDoc.RootElement,"window arm schema producer protocol_sha256");
  foreach(var k in new[]{"window","arm","schema","producer","protocol_sha256"})Need(!String.IsNullOrWhiteSpace(S(id,k))&&S(id,k)==S(idDoc.RootElement,k),"identity");
  var state=new State();var segments=h.GetProperty("segments");var tail=h.GetProperty("tail");Need(segments.ValueKind==JsonValueKind.Array&&segments.GetArrayLength()<=MaxSegments&&tail.ValueKind==JsonValueKind.Array&&tail.GetArrayLength()<Batch,"array_bound");
  long compressed=0;int index=0;var buffer=new byte[65536];var line=new byte[Line];
  foreach(var seg in segments.EnumerateArray()) {
   Keys(seg,"path first_sequence last_sequence event_count compressed_bytes uncompressed_bytes sha256 uncompressed_sha256");
   string rel=S(seg,"path");Need(rel=="authority-segment-"+(++index).ToString("D6")+".jsonl.gz","segment_order");
   ulong count=U(seg,"event_count");Need(count>0&&count<=Batch&&U(seg,"first_sequence")==state.Seq+1&&U(seg,"last_sequence")==state.Seq+count,"segment_range");
   long cb=I(seg,"compressed_bytes"),ub=I(seg,"uncompressed_bytes");Need(cb>0&&cb<=CompressedCap&&ub>0&&ub<=Batch*(Line+1L),"segment_bound");
   compressed=checked(compressed+cb);Need(compressed<=CompressedCap,"compressed_cap");
   string sh=S(seg,"sha256"),uh=S(seg,"uncompressed_sha256");Hex(sh);Hex(uh);string path=SafePath(directory,rel);
   Need(new FileInfo(path).Length==cb,"compressed_size");using(var f=File.OpenRead(path))Need(Hash(f)==sh,"compressed_hash");
   ulong before=state.Seq;long bytes=0;int used=0;using var raw=IncrementalHash.CreateHash(HashAlgorithmName.SHA256);
   using(var file=File.OpenRead(path))using(var gz=new GZipStream(file,CompressionMode.Decompress)) {
    int n;while((n=gz.Read(buffer,0,buffer.Length))>0){bytes+=n;Need(bytes<=ub,"uncompressed_bound");raw.AppendData(buffer,0,n);
     for(int j=0;j<n;j++){byte b=buffer[j];if(b==10){Need(used>0,"blank_line");using(var e=JsonDocument.Parse(new ReadOnlyMemory<byte>(line,0,used)))state.Event(e.RootElement);used=0;Need(state.Seq-before<=count,"segment_count");}
      else{Need(b!=13&&used<Line,"line_bound_or_cr");line[used++]=b;}}
    }
   }
   Need(used==0&&bytes==ub&&state.Seq-before==count,"truncated_or_count");Need(Convert.ToHexString(raw.GetHashAndReset()).ToLowerInvariant()==uh,"uncompressed_hash");
  }
  foreach(var e in tail.EnumerateArray()){Need(System.Text.Encoding.UTF8.GetByteCount(e.GetRawText())<=Line,"tail_bound");state.Event(e);}
  Need((ulong)compressed==U(h,"total_compressed_bytes")&&state.Seq==U(h,"event_count")&&state.Seq==U(h,"last_sequence"),"totals");
  Need(state.Active.Count==0&&state.Errors==U(h,"operation_failures"),"terminal_pair_or_errors");
  return JsonSerializer.Serialize(new {version="issue76-authority-stream-forensic-unsealed-v1",event_count=state.Seq,spans=state.Span,operation_failures=state.Errors,segments=index,total_compressed_bytes=compressed,peak_active_spans=state.Peak,qualified=false,quality_assessed=false});
 }
}
