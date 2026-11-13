import{J as It,g as ue,s as de,C as fe,D as he,a as me,b as ke,c as at,d as kt,e as ye,K as G,l as wt,k as ge,j as pe,E as ve,A as xe}from"./mermaid.core-BTbG1CfH.js";import"./index-aHqAdeLc.js";import{t as be,m as Te,a as we,l as _e,h as De,b as Nt,c as Bt,d as Ce,e as Se,f as Ee,g as Ae,i as Me,j as Ie,k as Le,n as qt,o as Ht,p as Xt,s as Gt,q as jt}from"./d3-scale-6E8tehIv.js";function Fe(t){return t}var gt=1,Dt=2,Ct=3,yt=4,Ut=1e-6;function Ye(t){return"translate("+t+",0)"}function We(t){return"translate(0,"+t+")"}function Ve(t){return e=>+t(e)}function Pe(t,e){return e=Math.max(0,t.bandwidth()-e*2)/2,t.round()&&(e=Math.round(e)),s=>+t(s)+e}function Oe(){return!this.__axis}function te(t,e){var s=[],i=null,a=null,h=6,d=6,w=3,C=typeof window<"u"&&window.devicePixelRatio>1?0:.5,D=t===gt||t===yt?-1:1,p=t===yt||t===Dt?"x":"y",A=t===gt||t===Ct?Ye:We;function _(v){var B=i??(e.ticks?e.ticks.apply(e,s):e.domain()),M=a??(e.tickFormat?e.tickFormat.apply(e,s):Fe),b=Math.max(h,0)+w,S=e.range(),L=+S[0]+C,F=+S[S.length-1]+C,R=(e.bandwidth?Pe:Ve)(e.copy(),C),z=v.selection?v.selection():v,H=z.selectAll(".domain").data([null]),O=z.selectAll(".tick").data(B,e).order(),m=O.exit(),T=O.enter().append("g").attr("class","tick"),x=O.select("line"),k=O.select("text");H=H.merge(H.enter().insert("path",".tick").attr("class","domain").attr("stroke","currentColor")),O=O.merge(T),x=x.merge(T.append("line").attr("stroke","currentColor").attr(p+"2",D*h)),k=k.merge(T.append("text").attr("fill","currentColor").attr(p,D*b).attr("dy",t===gt?"0em":t===Ct?"0.71em":"0.32em")),v!==z&&(H=H.transition(v),O=O.transition(v),x=x.transition(v),k=k.transition(v),m=m.transition(v).attr("opacity",Ut).attr("transform",function(n){return isFinite(n=R(n))?A(n+C):this.getAttribute("transform")}),T.attr("opacity",Ut).attr("transform",function(n){var c=this.parentNode.__axis;return A((c&&isFinite(c=c(n))?c:R(n))+C)})),m.remove(),H.attr("d",t===yt||t===Dt?d?"M"+D*d+","+L+"H"+C+"V"+F+"H"+D*d:"M"+C+","+L+"V"+F:d?"M"+L+","+D*d+"V"+C+"H"+F+"V"+D*d:"M"+L+","+C+"H"+F),O.attr("opacity",1).attr("transform",function(n){return A(R(n)+C)}),x.attr(p+"2",D*h),k.attr(p,D*b).text(M),z.filter(Oe).attr("fill","none").attr("font-size",10).attr("font-family","sans-serif").attr("text-anchor",t===Dt?"start":t===yt?"end":"middle"),z.each(function(){this.__axis=R})}return _.scale=function(v){return arguments.length?(e=v,_):e},_.ticks=function(){return s=Array.from(arguments),_},_.tickArguments=function(v){return arguments.length?(s=v==null?[]:Array.from(v),_):s.slice()},_.tickValues=function(v){return arguments.length?(i=v==null?null:Array.from(v),_):i&&i.slice()},_.tickFormat=function(v){return arguments.length?(a=v,_):a},_.tickSize=function(v){return arguments.length?(h=d=+v,_):h},_.tickSizeInner=function(v){return arguments.length?(h=+v,_):h},_.tickSizeOuter=function(v){return arguments.length?(d=+v,_):d},_.tickPadding=function(v){return arguments.length?(w=+v,_):w},_.offset=function(v){return arguments.length?(C=+v,_):C},_}function ze(t){return te(gt,t)}function Re(t){return te(Ct,t)}var pt={exports:{}},Ne=pt.exports,Zt;function Be(){return Zt||(Zt=1,(function(t,e){(function(s,i){t.exports=i()})(Ne,(function(){var s="day";return function(i,a,h){var d=function(D){return D.add(4-D.isoWeekday(),s)},w=a.prototype;w.isoWeekYear=function(){return d(this).year()},w.isoWeek=function(D){if(!this.$utils().u(D))return this.add(7*(D-this.isoWeek()),s);var p,A,_,v,B=d(this),M=(p=this.isoWeekYear(),A=this.$u,_=(A?h.utc:h)().year(p).startOf("year"),v=4-_.isoWeekday(),_.isoWeekday()>4&&(v+=7),_.add(v,s));return B.diff(M,"week")+1},w.isoWeekday=function(D){return this.$utils().u(D)?this.day()||7:this.day(this.day()%7?D:D-7)};var C=w.startOf;w.startOf=function(D,p){var A=this.$utils(),_=!!A.u(p)||p;return A.p(D)==="isoweek"?_?this.date(this.date()-(this.isoWeekday()-1)).startOf("day"):this.date(this.date()-1-(this.isoWeekday()-1)+7).endOf("day"):C.bind(this)(D,p)}}}))})(pt)),pt.exports}var qe=Be();const He=It(qe);var vt={exports:{}},Xe=vt.exports,Qt;function Ge(){return Qt||(Qt=1,(function(t,e){(function(s,i){t.exports=i()})(Xe,(function(){var s={LTS:"h:mm:ss A",LT:"h:mm A",L:"MM/DD/YYYY",LL:"MMMM D, YYYY",LLL:"MMMM D, YYYY h:mm A",LLLL:"dddd, MMMM D, YYYY h:mm A"},i=/(\[[^[]*\])|([-_:/.,()\s]+)|(A|a|Q|YYYY|YY?|ww?|MM?M?M?|Do|DD?|hh?|HH?|mm?|ss?|S{1,3}|z|ZZ?)/g,a=/\d/,h=/\d\d/,d=/\d\d?/,w=/\d*[^-_:/,()\s\d]+/,C={},D=function(b){return(b=+b)+(b>68?1900:2e3)},p=function(b){return function(S){this[b]=+S}},A=[/[+-]\d\d:?(\d\d)?|Z/,function(b){(this.zone||(this.zone={})).offset=(function(S){if(!S||S==="Z")return 0;var L=S.match(/([+-]|\d\d)/g),F=60*L[1]+(+L[2]||0);return F===0?0:L[0]==="+"?-F:F})(b)}],_=function(b){var S=C[b];return S&&(S.indexOf?S:S.s.concat(S.f))},v=function(b,S){var L,F=C.meridiem;if(F){for(var R=1;R<=24;R+=1)if(b.indexOf(F(R,0,S))>-1){L=R>12;break}}else L=b===(S?"pm":"PM");return L},B={A:[w,function(b){this.afternoon=v(b,!1)}],a:[w,function(b){this.afternoon=v(b,!0)}],Q:[a,function(b){this.month=3*(b-1)+1}],S:[a,function(b){this.milliseconds=100*+b}],SS:[h,function(b){this.milliseconds=10*+b}],SSS:[/\d{3}/,function(b){this.milliseconds=+b}],s:[d,p("seconds")],ss:[d,p("seconds")],m:[d,p("minutes")],mm:[d,p("minutes")],H:[d,p("hours")],h:[d,p("hours")],HH:[d,p("hours")],hh:[d,p("hours")],D:[d,p("day")],DD:[h,p("day")],Do:[w,function(b){var S=C.ordinal,L=b.match(/\d+/);if(this.day=L[0],S)for(var F=1;F<=31;F+=1)S(F).replace(/\[|\]/g,"")===b&&(this.day=F)}],w:[d,p("week")],ww:[h,p("week")],M:[d,p("month")],MM:[h,p("month")],MMM:[w,function(b){var S=_("months"),L=(_("monthsShort")||S.map((function(F){return F.slice(0,3)}))).indexOf(b)+1;if(L<1)throw new Error;this.month=L%12||L}],MMMM:[w,function(b){var S=_("months").indexOf(b)+1;if(S<1)throw new Error;this.month=S%12||S}],Y:[/[+-]?\d+/,p("year")],YY:[h,function(b){this.year=D(b)}],YYYY:[/\d{4}/,p("year")],Z:A,ZZ:A};function M(b){var S,L;S=b,L=C&&C.formats;for(var F=(b=S.replace(/(\[[^\]]+])|(LTS?|l{1,4}|L{1,4})/g,(function(x,k,n){var c=n&&n.toUpperCase();return k||L[n]||s[n]||L[c].replace(/(\[[^\]]+])|(MMMM|MM|DD|dddd)/g,(function(f,o,g){return o||g.slice(1)}))}))).match(i),R=F.length,z=0;z<R;z+=1){var H=F[z],O=B[H],m=O&&O[0],T=O&&O[1];F[z]=T?{regex:m,parser:T}:H.replace(/^\[|\]$/g,"")}return function(x){for(var k={},n=0,c=0;n<R;n+=1){var f=F[n];if(typeof f=="string")c+=f.length;else{var o=f.regex,g=f.parser,r=x.slice(c),P=o.exec(r)[0];g.call(k,P),x=x.replace(P,"")}}return(function(u){var l=u.afternoon;if(l!==void 0){var y=u.hours;l?y<12&&(u.hours+=12):y===12&&(u.hours=0),delete u.afternoon}})(k),k}}return function(b,S,L){L.p.customParseFormat=!0,b&&b.parseTwoDigitYear&&(D=b.parseTwoDigitYear);var F=S.prototype,R=F.parse;F.parse=function(z){var H=z.date,O=z.utc,m=z.args;this.$u=O;var T=m[1];if(typeof T=="string"){var x=m[2]===!0,k=m[3]===!0,n=x||k,c=m[2];k&&(c=m[2]),C=this.$locale(),!x&&c&&(C=L.Ls[c]),this.$d=(function(r,P,u,l){try{if(["x","X"].indexOf(P)>-1)return new Date((P==="X"?1e3:1)*r);var y=M(P)(r),V=y.year,E=y.month,Y=y.day,I=y.hours,W=y.minutes,nt=y.seconds,rt=y.milliseconds,ft=y.zone,ht=y.week,q=new Date,U=Y||(V||E?1:q.getDate()),X=V||q.getFullYear(),$=0;V&&!E||($=E>0?E-1:q.getMonth());var Z,tt=I||0,j=W||0,it=nt||0,et=rt||0;return ft?new Date(Date.UTC(X,$,U,tt,j,it,et+60*ft.offset*1e3)):u?new Date(Date.UTC(X,$,U,tt,j,it,et)):(Z=new Date(X,$,U,tt,j,it,et),ht&&(Z=l(Z).week(ht).toDate()),Z)}catch{return new Date("")}})(H,T,O,L),this.init(),c&&c!==!0&&(this.$L=this.locale(c).$L),n&&H!=this.format(T)&&(this.$d=new Date("")),C={}}else if(T instanceof Array)for(var f=T.length,o=1;o<=f;o+=1){m[1]=T[o-1];var g=L.apply(this,m);if(g.isValid()){this.$d=g.$d,this.$L=g.$L,this.init();break}o===f&&(this.$d=new Date(""))}else R.call(this,z)}}}))})(vt)),vt.exports}var je=Ge();const Ue=It(je);var xt={exports:{}},Ze=xt.exports,Jt;function Qe(){return Jt||(Jt=1,(function(t,e){(function(s,i){t.exports=i()})(Ze,(function(){return function(s,i){var a=i.prototype,h=a.format;a.format=function(d){var w=this,C=this.$locale();if(!this.isValid())return h.bind(this)(d);var D=this.$utils(),p=(d||"YYYY-MM-DDTHH:mm:ssZ").replace(/\[([^\]]+)]|Q|wo|ww|w|WW|W|zzz|z|gggg|GGGG|Do|X|x|k{1,2}|S/g,(function(A){switch(A){case"Q":return Math.ceil((w.$M+1)/3);case"Do":return C.ordinal(w.$D);case"gggg":return w.weekYear();case"GGGG":return w.isoWeekYear();case"wo":return C.ordinal(w.week(),"W");case"w":case"ww":return D.s(w.week(),A==="w"?1:2,"0");case"W":case"WW":return D.s(w.isoWeek(),A==="W"?1:2,"0");case"k":case"kk":return D.s(String(w.$H===0?24:w.$H),A==="k"?1:2,"0");case"X":return Math.floor(w.$d.getTime()/1e3);case"x":return w.$d.getTime();case"z":return"["+w.offsetName()+"]";case"zzz":return"["+w.offsetName("long")+"]";default:return A}}));return h.bind(this)(p)}}}))})(xt)),xt.exports}var Je=Qe();const Ke=It(Je);var St=(function(){var t=function(k,n,c,f){for(c=c||{},f=k.length;f--;c[k[f]]=n);return c},e=[6,8,10,12,13,14,15,16,17,18,19,20,21,22,23,24,25,26,27,28,30,32,33,35,37],s=[1,25],i=[1,26],a=[1,27],h=[1,28],d=[1,29],w=[1,30],C=[1,31],D=[1,9],p=[1,10],A=[1,11],_=[1,12],v=[1,13],B=[1,14],M=[1,15],b=[1,16],S=[1,18],L=[1,19],F=[1,20],R=[1,21],z=[1,22],H=[1,24],O=[1,32],m={trace:function(){},yy:{},symbols_:{error:2,start:3,gantt:4,document:5,EOF:6,line:7,SPACE:8,statement:9,NL:10,weekday:11,weekday_monday:12,weekday_tuesday:13,weekday_wednesday:14,weekday_thursday:15,weekday_friday:16,weekday_saturday:17,weekday_sunday:18,dateFormat:19,inclusiveEndDates:20,topAxis:21,axisFormat:22,tickInterval:23,excludes:24,includes:25,todayMarker:26,title:27,acc_title:28,acc_title_value:29,acc_descr:30,acc_descr_value:31,acc_descr_multiline_value:32,section:33,clickStatement:34,taskTxt:35,taskData:36,click:37,callbackname:38,callbackargs:39,href:40,clickStatementDebug:41,$accept:0,$end:1},terminals_:{2:"error",4:"gantt",6:"EOF",8:"SPACE",10:"NL",12:"weekday_monday",13:"weekday_tuesday",14:"weekday_wednesday",15:"weekday_thursday",16:"weekday_friday",17:"weekday_saturday",18:"weekday_sunday",19:"dateFormat",20:"inclusiveEndDates",21:"topAxis",22:"axisFormat",23:"tickInterval",24:"excludes",25:"includes",26:"todayMarker",27:"title",28:"acc_title",29:"acc_title_value",30:"acc_descr",31:"acc_descr_value",32:"acc_descr_multiline_value",33:"section",35:"taskTxt",36:"taskData",37:"click",38:"callbackname",39:"callbackargs",40:"href"},productions_:[0,[3,3],[5,0],[5,2],[7,2],[7,1],[7,1],[7,1],[11,1],[11,1],[11,1],[11,1],[11,1],[11,1],[11,1],[9,1],[9,1],[9,1],[9,1],[9,1],[9,1],[9,1],[9,1],[9,1],[9,1],[9,2],[9,2],[9,1],[9,1],[9,1],[9,2],[34,2],[34,3],[34,3],[34,4],[34,3],[34,4],[34,2],[41,2],[41,3],[41,3],[41,4],[41,3],[41,4],[41,2]],performAction:function(n,c,f,o,g,r,P){var u=r.length-1;switch(g){case 1:return r[u-1];case 2:this.$=[];break;case 3:r[u-1].push(r[u]),this.$=r[u-1];break;case 4:case 5:this.$=r[u];break;case 6:case 7:this.$=[];break;case 8:o.setWeekday("monday");break;case 9:o.setWeekday("tuesday");break;case 10:o.setWeekday("wednesday");break;case 11:o.setWeekday("thursday");break;case 12:o.setWeekday("friday");break;case 13:o.setWeekday("saturday");break;case 14:o.setWeekday("sunday");break;case 15:o.setDateFormat(r[u].substr(11)),this.$=r[u].substr(11);break;case 16:o.enableInclusiveEndDates(),this.$=r[u].substr(18);break;case 17:o.TopAxis(),this.$=r[u].substr(8);break;case 18:o.setAxisFormat(r[u].substr(11)),this.$=r[u].substr(11);break;case 19:o.setTickInterval(r[u].substr(13)),this.$=r[u].substr(13);break;case 20:o.setExcludes(r[u].substr(9)),this.$=r[u].substr(9);break;case 21:o.setIncludes(r[u].substr(9)),this.$=r[u].substr(9);break;case 22:o.setTodayMarker(r[u].substr(12)),this.$=r[u].substr(12);break;case 24:o.setDiagramTitle(r[u].substr(6)),this.$=r[u].substr(6);break;case 25:this.$=r[u].trim(),o.setAccTitle(this.$);break;case 26:case 27:this.$=r[u].trim(),o.setAccDescription(this.$);break;case 28:o.addSection(r[u].substr(8)),this.$=r[u].substr(8);break;case 30:o.addTask(r[u-1],r[u]),this.$="task";break;case 31:this.$=r[u-1],o.setClickEvent(r[u-1],r[u],null);break;case 32:this.$=r[u-2],o.setClickEvent(r[u-2],r[u-1],r[u]);break;case 33:this.$=r[u-2],o.setClickEvent(r[u-2],r[u-1],null),o.setLink(r[u-2],r[u]);break;case 34:this.$=r[u-3],o.setClickEvent(r[u-3],r[u-2],r[u-1]),o.setLink(r[u-3],r[u]);break;case 35:this.$=r[u-2],o.setClickEvent(r[u-2],r[u],null),o.setLink(r[u-2],r[u-1]);break;case 36:this.$=r[u-3],o.setClickEvent(r[u-3],r[u-1],r[u]),o.setLink(r[u-3],r[u-2]);break;case 37:this.$=r[u-1],o.setLink(r[u-1],r[u]);break;case 38:case 44:this.$=r[u-1]+" "+r[u];break;case 39:case 40:case 42:this.$=r[u-2]+" "+r[u-1]+" "+r[u];break;case 41:case 43:this.$=r[u-3]+" "+r[u-2]+" "+r[u-1]+" "+r[u];break}},table:[{3:1,4:[1,2]},{1:[3]},t(e,[2,2],{5:3}),{6:[1,4],7:5,8:[1,6],9:7,10:[1,8],11:17,12:s,13:i,14:a,15:h,16:d,17:w,18:C,19:D,20:p,21:A,22:_,23:v,24:B,25:M,26:b,27:S,28:L,30:F,32:R,33:z,34:23,35:H,37:O},t(e,[2,7],{1:[2,1]}),t(e,[2,3]),{9:33,11:17,12:s,13:i,14:a,15:h,16:d,17:w,18:C,19:D,20:p,21:A,22:_,23:v,24:B,25:M,26:b,27:S,28:L,30:F,32:R,33:z,34:23,35:H,37:O},t(e,[2,5]),t(e,[2,6]),t(e,[2,15]),t(e,[2,16]),t(e,[2,17]),t(e,[2,18]),t(e,[2,19]),t(e,[2,20]),t(e,[2,21]),t(e,[2,22]),t(e,[2,23]),t(e,[2,24]),{29:[1,34]},{31:[1,35]},t(e,[2,27]),t(e,[2,28]),t(e,[2,29]),{36:[1,36]},t(e,[2,8]),t(e,[2,9]),t(e,[2,10]),t(e,[2,11]),t(e,[2,12]),t(e,[2,13]),t(e,[2,14]),{38:[1,37],40:[1,38]},t(e,[2,4]),t(e,[2,25]),t(e,[2,26]),t(e,[2,30]),t(e,[2,31],{39:[1,39],40:[1,40]}),t(e,[2,37],{38:[1,41]}),t(e,[2,32],{40:[1,42]}),t(e,[2,33]),t(e,[2,35],{39:[1,43]}),t(e,[2,34]),t(e,[2,36])],defaultActions:{},parseError:function(n,c){if(c.recoverable)this.trace(n);else{var f=new Error(n);throw f.hash=c,f}},parse:function(n){var c=this,f=[0],o=[],g=[null],r=[],P=this.table,u="",l=0,y=0,V=2,E=1,Y=r.slice.call(arguments,1),I=Object.create(this.lexer),W={yy:{}};for(var nt in this.yy)Object.prototype.hasOwnProperty.call(this.yy,nt)&&(W.yy[nt]=this.yy[nt]);I.setInput(n,W.yy),W.yy.lexer=I,W.yy.parser=this,typeof I.yylloc>"u"&&(I.yylloc={});var rt=I.yylloc;r.push(rt);var ft=I.options&&I.options.ranges;typeof W.yy.parseError=="function"?this.parseError=W.yy.parseError:this.parseError=Object.getPrototypeOf(this).parseError;function ht(){var J;return J=o.pop()||I.lex()||E,typeof J!="number"&&(J instanceof Array&&(o=J,J=o.pop()),J=c.symbols_[J]||J),J}for(var q,U,X,$,Z={},tt,j,it,et;;){if(U=f[f.length-1],this.defaultActions[U]?X=this.defaultActions[U]:((q===null||typeof q>"u")&&(q=ht()),X=P[U]&&P[U][q]),typeof X>"u"||!X.length||!X[0]){var mt="";et=[];for(tt in P[U])this.terminals_[tt]&&tt>V&&et.push("'"+this.terminals_[tt]+"'");I.showPosition?mt="Parse error on line "+(l+1)+`:
`+I.showPosition()+`
Expecting `+et.join(", ")+", got '"+(this.terminals_[q]||q)+"'":mt="Parse error on line "+(l+1)+": Unexpected "+(q==E?"end of input":"'"+(this.terminals_[q]||q)+"'"),this.parseError(mt,{text:I.match,token:this.terminals_[q]||q,line:I.yylineno,loc:rt,expected:et})}if(X[0]instanceof Array&&X.length>1)throw new Error("Parse Error: multiple actions possible at state: "+U+", token: "+q);switch(X[0]){case 1:f.push(q),g.push(I.yytext),r.push(I.yylloc),f.push(X[1]),q=null,y=I.yyleng,u=I.yytext,l=I.yylineno,rt=I.yylloc;break;case 2:if(j=this.productions_[X[1]][1],Z.$=g[g.length-j],Z._$={first_line:r[r.length-(j||1)].first_line,last_line:r[r.length-1].last_line,first_column:r[r.length-(j||1)].first_column,last_column:r[r.length-1].last_column},ft&&(Z._$.range=[r[r.length-(j||1)].range[0],r[r.length-1].range[1]]),$=this.performAction.apply(Z,[u,y,l,W.yy,X[1],g,r].concat(Y)),typeof $<"u")return $;j&&(f=f.slice(0,-1*j*2),g=g.slice(0,-1*j),r=r.slice(0,-1*j)),f.push(this.productions_[X[1]][0]),g.push(Z.$),r.push(Z._$),it=P[f[f.length-2]][f[f.length-1]],f.push(it);break;case 3:return!0}}return!0}},T=(function(){var k={EOF:1,parseError:function(c,f){if(this.yy.parser)this.yy.parser.parseError(c,f);else throw new Error(c)},setInput:function(n,c){return this.yy=c||this.yy||{},this._input=n,this._more=this._backtrack=this.done=!1,this.yylineno=this.yyleng=0,this.yytext=this.matched=this.match="",this.conditionStack=["INITIAL"],this.yylloc={first_line:1,first_column:0,last_line:1,last_column:0},this.options.ranges&&(this.yylloc.range=[0,0]),this.offset=0,this},input:function(){var n=this._input[0];this.yytext+=n,this.yyleng++,this.offset++,this.match+=n,this.matched+=n;var c=n.match(/(?:\r\n?|\n).*/g);return c?(this.yylineno++,this.yylloc.last_line++):this.yylloc.last_column++,this.options.ranges&&this.yylloc.range[1]++,this._input=this._input.slice(1),n},unput:function(n){var c=n.length,f=n.split(/(?:\r\n?|\n)/g);this._input=n+this._input,this.yytext=this.yytext.substr(0,this.yytext.length-c),this.offset-=c;var o=this.match.split(/(?:\r\n?|\n)/g);this.match=this.match.substr(0,this.match.length-1),this.matched=this.matched.substr(0,this.matched.length-1),f.length-1&&(this.yylineno-=f.length-1);var g=this.yylloc.range;return this.yylloc={first_line:this.yylloc.first_line,last_line:this.yylineno+1,first_column:this.yylloc.first_column,last_column:f?(f.length===o.length?this.yylloc.first_column:0)+o[o.length-f.length].length-f[0].length:this.yylloc.first_column-c},this.options.ranges&&(this.yylloc.range=[g[0],g[0]+this.yyleng-c]),this.yyleng=this.yytext.length,this},more:function(){return this._more=!0,this},reject:function(){if(this.options.backtrack_lexer)this._backtrack=!0;else return this.parseError("Lexical error on line "+(this.yylineno+1)+`. You can only invoke reject() in the lexer when the lexer is of the backtracking persuasion (options.backtrack_lexer = true).
`+this.showPosition(),{text:"",token:null,line:this.yylineno});return this},less:function(n){this.unput(this.match.slice(n))},pastInput:function(){var n=this.matched.substr(0,this.matched.length-this.match.length);return(n.length>20?"...":"")+n.substr(-20).replace(/\n/g,"")},upcomingInput:function(){var n=this.match;return n.length<20&&(n+=this._input.substr(0,20-n.length)),(n.substr(0,20)+(n.length>20?"...":"")).replace(/\n/g,"")},showPosition:function(){var n=this.pastInput(),c=new Array(n.length+1).join("-");return n+this.upcomingInput()+`
`+c+"^"},test_match:function(n,c){var f,o,g;if(this.options.backtrack_lexer&&(g={yylineno:this.yylineno,yylloc:{first_line:this.yylloc.first_line,last_line:this.last_line,first_column:this.yylloc.first_column,last_column:this.yylloc.last_column},yytext:this.yytext,match:this.match,matches:this.matches,matched:this.matched,yyleng:this.yyleng,offset:this.offset,_more:this._more,_input:this._input,yy:this.yy,conditionStack:this.conditionStack.slice(0),done:this.done},this.options.ranges&&(g.yylloc.range=this.yylloc.range.slice(0))),o=n[0].match(/(?:\r\n?|\n).*/g),o&&(this.yylineno+=o.length),this.yylloc={first_line:this.yylloc.last_line,last_line:this.yylineno+1,first_column:this.yylloc.last_column,last_column:o?o[o.length-1].length-o[o.length-1].match(/\r?\n?/)[0].length:this.yylloc.last_column+n[0].length},this.yytext+=n[0],this.match+=n[0],this.matches=n,this.yyleng=this.yytext.length,this.options.ranges&&(this.yylloc.range=[this.offset,this.offset+=this.yyleng]),this._more=!1,this._backtrack=!1,this._input=this._input.slice(n[0].length),this.matched+=n[0],f=this.performAction.call(this,this.yy,this,c,this.conditionStack[this.conditionStack.length-1]),this.done&&this._input&&(this.done=!1),f)return f;if(this._backtrack){for(var r in g)this[r]=g[r];return!1}return!1},next:function(){if(this.done)return this.EOF;this._input||(this.done=!0);var n,c,f,o;this._more||(this.yytext="",this.match="");for(var g=this._currentRules(),r=0;r<g.length;r++)if(f=this._input.match(this.rules[g[r]]),f&&(!c||f[0].length>c[0].length)){if(c=f,o=r,this.options.backtrack_lexer){if(n=this.test_match(f,g[r]),n!==!1)return n;if(this._backtrack){c=!1;continue}else return!1}else if(!this.options.flex)break}return c?(n=this.test_match(c,g[o]),n!==!1?n:!1):this._input===""?this.EOF:this.parseError("Lexical error on line "+(this.yylineno+1)+`. Unrecognized text.
`+this.showPosition(),{text:"",token:null,line:this.yylineno})},lex:function(){var c=this.next();return c||this.lex()},begin:function(c){this.conditionStack.push(c)},popState:function(){var c=this.conditionStack.length-1;return c>0?this.conditionStack.pop():this.conditionStack[0]},_currentRules:function(){return this.conditionStack.length&&this.conditionStack[this.conditionStack.length-1]?this.conditions[this.conditionStack[this.conditionStack.length-1]].rules:this.conditions.INITIAL.rules},topState:function(c){return c=this.conditionStack.length-1-Math.abs(c||0),c>=0?this.conditionStack[c]:"INITIAL"},pushState:function(c){this.begin(c)},stateStackSize:function(){return this.conditionStack.length},options:{"case-insensitive":!0},performAction:function(c,f,o,g){switch(o){case 0:return this.begin("open_directive"),"open_directive";case 1:return this.begin("acc_title"),28;case 2:return this.popState(),"acc_title_value";case 3:return this.begin("acc_descr"),30;case 4:return this.popState(),"acc_descr_value";case 5:this.begin("acc_descr_multiline");break;case 6:this.popState();break;case 7:return"acc_descr_multiline_value";case 8:break;case 9:break;case 10:break;case 11:return 10;case 12:break;case 13:break;case 14:this.begin("href");break;case 15:this.popState();break;case 16:return 40;case 17:this.begin("callbackname");break;case 18:this.popState();break;case 19:this.popState(),this.begin("callbackargs");break;case 20:return 38;case 21:this.popState();break;case 22:return 39;case 23:this.begin("click");break;case 24:this.popState();break;case 25:return 37;case 26:return 4;case 27:return 19;case 28:return 20;case 29:return 21;case 30:return 22;case 31:return 23;case 32:return 25;case 33:return 24;case 34:return 26;case 35:return 12;case 36:return 13;case 37:return 14;case 38:return 15;case 39:return 16;case 40:return 17;case 41:return 18;case 42:return"date";case 43:return 27;case 44:return"accDescription";case 45:return 33;case 46:return 35;case 47:return 36;case 48:return":";case 49:return 6;case 50:return"INVALID"}},rules:[/^(?:%%\{)/i,/^(?:accTitle\s*:\s*)/i,/^(?:(?!\n||)*[^\n]*)/i,/^(?:accDescr\s*:\s*)/i,/^(?:(?!\n||)*[^\n]*)/i,/^(?:accDescr\s*\{\s*)/i,/^(?:[\}])/i,/^(?:[^\}]*)/i,/^(?:%%(?!\{)*[^\n]*)/i,/^(?:[^\}]%%*[^\n]*)/i,/^(?:%%*[^\n]*[\n]*)/i,/^(?:[\n]+)/i,/^(?:\s+)/i,/^(?:%[^\n]*)/i,/^(?:href[\s]+["])/i,/^(?:["])/i,/^(?:[^"]*)/i,/^(?:call[\s]+)/i,/^(?:\([\s]*\))/i,/^(?:\()/i,/^(?:[^(]*)/i,/^(?:\))/i,/^(?:[^)]*)/i,/^(?:click[\s]+)/i,/^(?:[\s\n])/i,/^(?:[^\s\n]*)/i,/^(?:gantt\b)/i,/^(?:dateFormat\s[^#\n;]+)/i,/^(?:inclusiveEndDates\b)/i,/^(?:topAxis\b)/i,/^(?:axisFormat\s[^#\n;]+)/i,/^(?:tickInterval\s[^#\n;]+)/i,/^(?:includes\s[^#\n;]+)/i,/^(?:excludes\s[^#\n;]+)/i,/^(?:todayMarker\s[^\n;]+)/i,/^(?:weekday\s+monday\b)/i,/^(?:weekday\s+tuesday\b)/i,/^(?:weekday\s+wednesday\b)/i,/^(?:weekday\s+thursday\b)/i,/^(?:weekday\s+friday\b)/i,/^(?:weekday\s+saturday\b)/i,/^(?:weekday\s+sunday\b)/i,/^(?:\d\d\d\d-\d\d-\d\d\b)/i,/^(?:title\s[^\n]+)/i,/^(?:accDescription\s[^#\n;]+)/i,/^(?:section\s[^\n]+)/i,/^(?:[^:\n]+)/i,/^(?::[^#\n;]+)/i,/^(?::)/i,/^(?:$)/i,/^(?:.)/i],conditions:{acc_descr_multiline:{rules:[6,7],inclusive:!1},acc_descr:{rules:[4],inclusive:!1},acc_title:{rules:[2],inclusive:!1},callbackargs:{rules:[21,22],inclusive:!1},callbackname:{rules:[18,19,20],inclusive:!1},href:{rules:[15,16],inclusive:!1},click:{rules:[24,25],inclusive:!1},INITIAL:{rules:[0,1,3,5,8,9,10,11,12,13,14,17,23,26,27,28,29,30,31,32,33,34,35,36,37,38,39,40,41,42,43,44,45,46,47,48,49,50],inclusive:!0}}};return k})();m.lexer=T;function x(){this.yy={}}return x.prototype=m,m.Parser=x,new x})();St.parser=St;const $e=St;G.extend(He);G.extend(Ue);G.extend(Ke);let Q="",Lt="",Ft,Yt="",lt=[],ut=[],Wt={},Vt=[],_t=[],ct="",Pt="";const ee=["active","done","crit","milestone"];let Ot=[],dt=!1,zt=!1,Rt="sunday",Et=0;const tn=function(){Vt=[],_t=[],ct="",Ot=[],bt=0,Mt=void 0,Tt=void 0,N=[],Q="",Lt="",Pt="",Ft=void 0,Yt="",lt=[],ut=[],dt=!1,zt=!1,Et=0,Wt={},ve(),Rt="sunday"},en=function(t){Lt=t},nn=function(){return Lt},rn=function(t){Ft=t},sn=function(){return Ft},an=function(t){Yt=t},on=function(){return Yt},cn=function(t){Q=t},ln=function(){dt=!0},un=function(){return dt},dn=function(){zt=!0},fn=function(){return zt},hn=function(t){Pt=t},mn=function(){return Pt},kn=function(){return Q},yn=function(t){lt=t.toLowerCase().split(/[\s,]+/)},gn=function(){return lt},pn=function(t){ut=t.toLowerCase().split(/[\s,]+/)},vn=function(){return ut},xn=function(){return Wt},bn=function(t){ct=t,Vt.push(t)},Tn=function(){return Vt},wn=function(){let t=Kt();const e=10;let s=0;for(;!t&&s<e;)t=Kt(),s++;return _t=N,_t},ne=function(t,e,s,i){return i.includes(t.format(e.trim()))?!1:t.isoWeekday()>=6&&s.includes("weekends")||s.includes(t.format("dddd").toLowerCase())?!0:s.includes(t.format(e.trim()))},_n=function(t){Rt=t},Dn=function(){return Rt},re=function(t,e,s,i){if(!s.length||t.manualEndTime)return;let a;t.startTime instanceof Date?a=G(t.startTime):a=G(t.startTime,e,!0),a=a.add(1,"d");let h;t.endTime instanceof Date?h=G(t.endTime):h=G(t.endTime,e,!0);const[d,w]=Cn(a,h,e,s,i);t.endTime=d.toDate(),t.renderEndTime=w},Cn=function(t,e,s,i,a){let h=!1,d=null;for(;t<=e;)h||(d=e.toDate()),h=ne(t,s,i,a),h&&(e=e.add(1,"d")),t=t.add(1,"d");return[e,d]},At=function(t,e,s){s=s.trim();const a=/^after\s+(?<ids>[\d\w- ]+)/.exec(s);if(a!==null){let d=null;for(const C of a.groups.ids.split(" ")){let D=st(C);D!==void 0&&(!d||D.endTime>d.endTime)&&(d=D)}if(d)return d.endTime;const w=new Date;return w.setHours(0,0,0,0),w}let h=G(s,e.trim(),!0);if(h.isValid())return h.toDate();{wt.debug("Invalid date:"+s),wt.debug("With date format:"+e.trim());const d=new Date(s);if(d===void 0||isNaN(d.getTime())||d.getFullYear()<-1e4||d.getFullYear()>1e4)throw new Error("Invalid date:"+s);return d}},se=function(t){const e=/^(\d+(?:\.\d+)?)([Mdhmswy]|ms)$/.exec(t.trim());return e!==null?[Number.parseFloat(e[1]),e[2]]:[NaN,"ms"]},ie=function(t,e,s,i=!1){s=s.trim();const h=/^until\s+(?<ids>[\d\w- ]+)/.exec(s);if(h!==null){let p=null;for(const _ of h.groups.ids.split(" ")){let v=st(_);v!==void 0&&(!p||v.startTime<p.startTime)&&(p=v)}if(p)return p.startTime;const A=new Date;return A.setHours(0,0,0,0),A}let d=G(s,e.trim(),!0);if(d.isValid())return i&&(d=d.add(1,"d")),d.toDate();let w=G(t);const[C,D]=se(s);if(!Number.isNaN(C)){const p=w.add(C,D);p.isValid()&&(w=p)}return w.toDate()};let bt=0;const ot=function(t){return t===void 0?(bt=bt+1,"task"+bt):t},Sn=function(t,e){let s;e.substr(0,1)===":"?s=e.substr(1,e.length):s=e;const i=s.split(","),a={};le(i,a,ee);for(let d=0;d<i.length;d++)i[d]=i[d].trim();let h="";switch(i.length){case 1:a.id=ot(),a.startTime=t.endTime,h=i[0];break;case 2:a.id=ot(),a.startTime=At(void 0,Q,i[0]),h=i[1];break;case 3:a.id=ot(i[0]),a.startTime=At(void 0,Q,i[1]),h=i[2];break}return h&&(a.endTime=ie(a.startTime,Q,h,dt),a.manualEndTime=G(h,"YYYY-MM-DD",!0).isValid(),re(a,Q,ut,lt)),a},En=function(t,e){let s;e.substr(0,1)===":"?s=e.substr(1,e.length):s=e;const i=s.split(","),a={};le(i,a,ee);for(let h=0;h<i.length;h++)i[h]=i[h].trim();switch(i.length){case 1:a.id=ot(),a.startTime={type:"prevTaskEnd",id:t},a.endTime={data:i[0]};break;case 2:a.id=ot(),a.startTime={type:"getStartDate",startData:i[0]},a.endTime={data:i[1]};break;case 3:a.id=ot(i[0]),a.startTime={type:"getStartDate",startData:i[1]},a.endTime={data:i[2]};break}return a};let Mt,Tt,N=[];const ae={},An=function(t,e){const s={section:ct,type:ct,processed:!1,manualEndTime:!1,renderEndTime:null,raw:{data:e},task:t,classes:[]},i=En(Tt,e);s.raw.startTime=i.startTime,s.raw.endTime=i.endTime,s.id=i.id,s.prevTaskId=Tt,s.active=i.active,s.done=i.done,s.crit=i.crit,s.milestone=i.milestone,s.order=Et,Et++;const a=N.push(s);Tt=s.id,ae[s.id]=a-1},st=function(t){const e=ae[t];return N[e]},Mn=function(t,e){const s={section:ct,type:ct,description:t,task:t,classes:[]},i=Sn(Mt,e);s.startTime=i.startTime,s.endTime=i.endTime,s.id=i.id,s.active=i.active,s.done=i.done,s.crit=i.crit,s.milestone=i.milestone,Mt=s,_t.push(s)},Kt=function(){const t=function(s){const i=N[s];let a="";switch(N[s].raw.startTime.type){case"prevTaskEnd":{const h=st(i.prevTaskId);i.startTime=h.endTime;break}case"getStartDate":a=At(void 0,Q,N[s].raw.startTime.startData),a&&(N[s].startTime=a);break}return N[s].startTime&&(N[s].endTime=ie(N[s].startTime,Q,N[s].raw.endTime.data,dt),N[s].endTime&&(N[s].processed=!0,N[s].manualEndTime=G(N[s].raw.endTime.data,"YYYY-MM-DD",!0).isValid(),re(N[s],Q,ut,lt))),N[s].processed};let e=!0;for(const[s,i]of N.entries())t(s),e=e&&i.processed;return e},In=function(t,e){let s=e;at().securityLevel!=="loose"&&(s=pe.sanitizeUrl(e)),t.split(",").forEach(function(i){st(i)!==void 0&&(ce(i,()=>{window.open(s,"_self")}),Wt[i]=s)}),oe(t,"clickable")},oe=function(t,e){t.split(",").forEach(function(s){let i=st(s);i!==void 0&&i.classes.push(e)})},Ln=function(t,e,s){if(at().securityLevel!=="loose"||e===void 0)return;let i=[];if(typeof s=="string"){i=s.split(/,(?=(?:(?:[^"]*"){2})*[^"]*$)/);for(let h=0;h<i.length;h++){let d=i[h].trim();d.charAt(0)==='"'&&d.charAt(d.length-1)==='"'&&(d=d.substr(1,d.length-2)),i[h]=d}}i.length===0&&i.push(t),st(t)!==void 0&&ce(t,()=>{xe.runFunc(e,...i)})},ce=function(t,e){Ot.push(function(){const s=document.querySelector(`[id="${t}"]`);s!==null&&s.addEventListener("click",function(){e()})},function(){const s=document.querySelector(`[id="${t}-text"]`);s!==null&&s.addEventListener("click",function(){e()})})},Fn=function(t,e,s){t.split(",").forEach(function(i){Ln(i,e,s)}),oe(t,"clickable")},Yn=function(t){Ot.forEach(function(e){e(t)})},Wn={getConfig:()=>at().gantt,clear:tn,setDateFormat:cn,getDateFormat:kn,enableInclusiveEndDates:ln,endDatesAreInclusive:un,enableTopAxis:dn,topAxisEnabled:fn,setAxisFormat:en,getAxisFormat:nn,setTickInterval:rn,getTickInterval:sn,setTodayMarker:an,getTodayMarker:on,setAccTitle:ke,getAccTitle:me,setDiagramTitle:he,getDiagramTitle:fe,setDisplayMode:hn,getDisplayMode:mn,setAccDescription:de,getAccDescription:ue,addSection:bn,getSections:Tn,getTasks:wn,addTask:An,findTaskById:st,addTaskOrg:Mn,setIncludes:yn,getIncludes:gn,setExcludes:pn,getExcludes:vn,setClickEvent:Fn,setLink:In,getLinks:xn,bindFunctions:Yn,parseDuration:se,isInvalidDate:ne,setWeekday:_n,getWeekday:Dn};function le(t,e,s){let i=!0;for(;i;)i=!1,s.forEach(function(a){const h="^\\s*"+a+"\\s*$",d=new RegExp(h);t[0].match(d)&&(e[a]=!0,t.shift(1),i=!0)})}const Vn=function(){wt.debug("Something is calling, setConf, remove the call")},$t={monday:Le,tuesday:Ie,wednesday:Me,thursday:Ae,friday:Ee,saturday:Se,sunday:Ce},Pn=(t,e)=>{let s=[...t].map(()=>-1/0),i=[...t].sort((h,d)=>h.startTime-d.startTime||h.order-d.order),a=0;for(const h of i)for(let d=0;d<s.length;d++)if(h.startTime>=s[d]){s[d]=h.endTime,h.order=d+e,d>a&&(a=d);break}return a};let K;const On=function(t,e,s,i){const a=at().gantt,h=at().securityLevel;let d;h==="sandbox"&&(d=kt("#i"+e));const w=h==="sandbox"?kt(d.nodes()[0].contentDocument.body):kt("body"),C=h==="sandbox"?d.nodes()[0].contentDocument:document,D=C.getElementById(e);K=D.parentElement.offsetWidth,K===void 0&&(K=1200),a.useWidth!==void 0&&(K=a.useWidth);const p=i.db.getTasks();let A=[];for(const m of p)A.push(m.type);A=O(A);const _={};let v=2*a.topPadding;if(i.db.getDisplayMode()==="compact"||a.displayMode==="compact"){const m={};for(const x of p)m[x.section]===void 0?m[x.section]=[x]:m[x.section].push(x);let T=0;for(const x of Object.keys(m)){const k=Pn(m[x],T)+1;T+=k,v+=k*(a.barHeight+a.barGap),_[x]=k}}else{v+=p.length*(a.barHeight+a.barGap);for(const m of A)_[m]=p.filter(T=>T.type===m).length}D.setAttribute("viewBox","0 0 "+K+" "+v);const B=w.select(`[id="${e}"]`),M=be().domain([Te(p,function(m){return m.startTime}),we(p,function(m){return m.endTime})]).rangeRound([0,K-a.leftPadding-a.rightPadding]);function b(m,T){const x=m.startTime,k=T.startTime;let n=0;return x>k?n=1:x<k&&(n=-1),n}p.sort(b),S(p,K,v),ye(B,v,K,a.useMaxWidth),B.append("text").text(i.db.getDiagramTitle()).attr("x",K/2).attr("y",a.titleTopMargin).attr("class","titleText");function S(m,T,x){const k=a.barHeight,n=k+a.barGap,c=a.topPadding,f=a.leftPadding,o=_e().domain([0,A.length]).range(["#00B9FA","#F95002"]).interpolate(De);F(n,c,f,T,x,m,i.db.getExcludes(),i.db.getIncludes()),R(f,c,T,x),L(m,n,c,f,k,o,T),z(n,c),H(f,c,T,x)}function L(m,T,x,k,n,c,f){const g=[...new Set(m.map(l=>l.order))].map(l=>m.find(y=>y.order===l));B.append("g").selectAll("rect").data(g).enter().append("rect").attr("x",0).attr("y",function(l,y){return y=l.order,y*T+x-2}).attr("width",function(){return f-a.rightPadding/2}).attr("height",T).attr("class",function(l){for(const[y,V]of A.entries())if(l.type===V)return"section section"+y%a.numberSectionStyles;return"section section0"});const r=B.append("g").selectAll("rect").data(m).enter(),P=i.db.getLinks();if(r.append("rect").attr("id",function(l){return l.id}).attr("rx",3).attr("ry",3).attr("x",function(l){return l.milestone?M(l.startTime)+k+.5*(M(l.endTime)-M(l.startTime))-.5*n:M(l.startTime)+k}).attr("y",function(l,y){return y=l.order,y*T+x}).attr("width",function(l){return l.milestone?n:M(l.renderEndTime||l.endTime)-M(l.startTime)}).attr("height",n).attr("transform-origin",function(l,y){return y=l.order,(M(l.startTime)+k+.5*(M(l.endTime)-M(l.startTime))).toString()+"px "+(y*T+x+.5*n).toString()+"px"}).attr("class",function(l){const y="task";let V="";l.classes.length>0&&(V=l.classes.join(" "));let E=0;for(const[I,W]of A.entries())l.type===W&&(E=I%a.numberSectionStyles);let Y="";return l.active?l.crit?Y+=" activeCrit":Y=" active":l.done?l.crit?Y=" doneCrit":Y=" done":l.crit&&(Y+=" crit"),Y.length===0&&(Y=" task"),l.milestone&&(Y=" milestone "+Y),Y+=E,Y+=" "+V,y+Y}),r.append("text").attr("id",function(l){return l.id+"-text"}).text(function(l){return l.task}).attr("font-size",a.fontSize).attr("x",function(l){let y=M(l.startTime),V=M(l.renderEndTime||l.endTime);l.milestone&&(y+=.5*(M(l.endTime)-M(l.startTime))-.5*n),l.milestone&&(V=y+n);const E=this.getBBox().width;return E>V-y?V+E+1.5*a.leftPadding>f?y+k-5:V+k+5:(V-y)/2+y+k}).attr("y",function(l,y){return y=l.order,y*T+a.barHeight/2+(a.fontSize/2-2)+x}).attr("text-height",n).attr("class",function(l){const y=M(l.startTime);let V=M(l.endTime);l.milestone&&(V=y+n);const E=this.getBBox().width;let Y="";l.classes.length>0&&(Y=l.classes.join(" "));let I=0;for(const[nt,rt]of A.entries())l.type===rt&&(I=nt%a.numberSectionStyles);let W="";return l.active&&(l.crit?W="activeCritText"+I:W="activeText"+I),l.done?l.crit?W=W+" doneCritText"+I:W=W+" doneText"+I:l.crit&&(W=W+" critText"+I),l.milestone&&(W+=" milestoneText"),E>V-y?V+E+1.5*a.leftPadding>f?Y+" taskTextOutsideLeft taskTextOutside"+I+" "+W:Y+" taskTextOutsideRight taskTextOutside"+I+" "+W+" width-"+E:Y+" taskText taskText"+I+" "+W+" width-"+E}),at().securityLevel==="sandbox"){let l;l=kt("#i"+e);const y=l.nodes()[0].contentDocument;r.filter(function(V){return P[V.id]!==void 0}).each(function(V){var E=y.querySelector("#"+V.id),Y=y.querySelector("#"+V.id+"-text");const I=E.parentNode;var W=y.createElement("a");W.setAttribute("xlink:href",P[V.id]),W.setAttribute("target","_top"),I.appendChild(W),W.appendChild(E),W.appendChild(Y)})}}function F(m,T,x,k,n,c,f,o){if(f.length===0&&o.length===0)return;let g,r;for(const{startTime:E,endTime:Y}of c)(g===void 0||E<g)&&(g=E),(r===void 0||Y>r)&&(r=Y);if(!g||!r)return;if(G(r).diff(G(g),"year")>5){wt.warn("The difference between the min and max time is more than 5 years. This will cause performance issues. Skipping drawing exclude days.");return}const P=i.db.getDateFormat(),u=[];let l=null,y=G(g);for(;y.valueOf()<=r;)i.db.isInvalidDate(y,P,f,o)?l?l.end=y:l={start:y,end:y}:l&&(u.push(l),l=null),y=y.add(1,"d");B.append("g").selectAll("rect").data(u).enter().append("rect").attr("id",function(E){return"exclude-"+E.start.format("YYYY-MM-DD")}).attr("x",function(E){return M(E.start)+x}).attr("y",a.gridLineStartPadding).attr("width",function(E){const Y=E.end.add(1,"day");return M(Y)-M(E.start)}).attr("height",n-T-a.gridLineStartPadding).attr("transform-origin",function(E,Y){return(M(E.start)+x+.5*(M(E.end)-M(E.start))).toString()+"px "+(Y*m+.5*n).toString()+"px"}).attr("class","exclude-range")}function R(m,T,x,k){let n=Re(M).tickSize(-k+T+a.gridLineStartPadding).tickFormat(Nt(i.db.getAxisFormat()||a.axisFormat||"%Y-%m-%d"));const f=/^([1-9]\d*)(millisecond|second|minute|hour|day|week|month)$/.exec(i.db.getTickInterval()||a.tickInterval);if(f!==null){const o=f[1],g=f[2],r=i.db.getWeekday()||a.weekday;switch(g){case"millisecond":n.ticks(jt.every(o));break;case"second":n.ticks(Gt.every(o));break;case"minute":n.ticks(Xt.every(o));break;case"hour":n.ticks(Ht.every(o));break;case"day":n.ticks(qt.every(o));break;case"week":n.ticks($t[r].every(o));break;case"month":n.ticks(Bt.every(o));break}}if(B.append("g").attr("class","grid").attr("transform","translate("+m+", "+(k-50)+")").call(n).selectAll("text").style("text-anchor","middle").attr("fill","#000").attr("stroke","none").attr("font-size",10).attr("dy","1em"),i.db.topAxisEnabled()||a.topAxis){let o=ze(M).tickSize(-k+T+a.gridLineStartPadding).tickFormat(Nt(i.db.getAxisFormat()||a.axisFormat||"%Y-%m-%d"));if(f!==null){const g=f[1],r=f[2],P=i.db.getWeekday()||a.weekday;switch(r){case"millisecond":o.ticks(jt.every(g));break;case"second":o.ticks(Gt.every(g));break;case"minute":o.ticks(Xt.every(g));break;case"hour":o.ticks(Ht.every(g));break;case"day":o.ticks(qt.every(g));break;case"week":o.ticks($t[P].every(g));break;case"month":o.ticks(Bt.every(g));break}}B.append("g").attr("class","grid").attr("transform","translate("+m+", "+T+")").call(o).selectAll("text").style("text-anchor","middle").attr("fill","#000").attr("stroke","none").attr("font-size",10)}}function z(m,T){let x=0;const k=Object.keys(_).map(n=>[n,_[n]]);B.append("g").selectAll("text").data(k).enter().append(function(n){const c=n[0].split(ge.lineBreakRegex),f=-(c.length-1)/2,o=C.createElementNS("http://www.w3.org/2000/svg","text");o.setAttribute("dy",f+"em");for(const[g,r]of c.entries()){const P=C.createElementNS("http://www.w3.org/2000/svg","tspan");P.setAttribute("alignment-baseline","central"),P.setAttribute("x","10"),g>0&&P.setAttribute("dy","1em"),P.textContent=r,o.appendChild(P)}return o}).attr("x",10).attr("y",function(n,c){if(c>0)for(let f=0;f<c;f++)return x+=k[c-1][1],n[1]*m/2+x*m+T;else return n[1]*m/2+T}).attr("font-size",a.sectionFontSize).attr("class",function(n){for(const[c,f]of A.entries())if(n[0]===f)return"sectionTitle sectionTitle"+c%a.numberSectionStyles;return"sectionTitle"})}function H(m,T,x,k){const n=i.db.getTodayMarker();if(n==="off")return;const c=B.append("g").attr("class","today"),f=new Date,o=c.append("line");o.attr("x1",M(f)+m).attr("x2",M(f)+m).attr("y1",a.titleTopMargin).attr("y2",k-a.titleTopMargin).attr("class","today"),n!==""&&o.attr("style",n.replace(/,/g,";"))}function O(m){const T={},x=[];for(let k=0,n=m.length;k<n;++k)Object.prototype.hasOwnProperty.call(T,m[k])||(T[m[k]]=!0,x.push(m[k]));return x}},zn={setConf:Vn,draw:On},Rn=t=>`
  .mermaid-main-font {
    font-family: var(--mermaid-font-family, "trebuchet ms", verdana, arial, sans-serif);
  }

  .exclude-range {
    fill: ${t.excludeBkgColor};
  }

  .section {
    stroke: none;
    opacity: 0.2;
  }

  .section0 {
    fill: ${t.sectionBkgColor};
  }

  .section2 {
    fill: ${t.sectionBkgColor2};
  }

  .section1,
  .section3 {
    fill: ${t.altSectionBkgColor};
    opacity: 0.2;
  }

  .sectionTitle0 {
    fill: ${t.titleColor};
  }

  .sectionTitle1 {
    fill: ${t.titleColor};
  }

  .sectionTitle2 {
    fill: ${t.titleColor};
  }

  .sectionTitle3 {
    fill: ${t.titleColor};
  }

  .sectionTitle {
    text-anchor: start;
    font-family: var(--mermaid-font-family, "trebuchet ms", verdana, arial, sans-serif);
  }


  /* Grid and axis */

  .grid .tick {
    stroke: ${t.gridColor};
    opacity: 0.8;
    shape-rendering: crispEdges;
  }

  .grid .tick text {
    font-family: ${t.fontFamily};
    fill: ${t.textColor};
  }

  .grid path {
    stroke-width: 0;
  }


  /* Today line */

  .today {
    fill: none;
    stroke: ${t.todayLineColor};
    stroke-width: 2px;
  }


  /* Task styling */

  /* Default task */

  .task {
    stroke-width: 2;
  }

  .taskText {
    text-anchor: middle;
    font-family: var(--mermaid-font-family, "trebuchet ms", verdana, arial, sans-serif);
  }

  .taskTextOutsideRight {
    fill: ${t.taskTextDarkColor};
    text-anchor: start;
    font-family: var(--mermaid-font-family, "trebuchet ms", verdana, arial, sans-serif);
  }

  .taskTextOutsideLeft {
    fill: ${t.taskTextDarkColor};
    text-anchor: end;
  }


  /* Special case clickable */

  .task.clickable {
    cursor: pointer;
  }

  .taskText.clickable {
    cursor: pointer;
    fill: ${t.taskTextClickableColor} !important;
    font-weight: bold;
  }

  .taskTextOutsideLeft.clickable {
    cursor: pointer;
    fill: ${t.taskTextClickableColor} !important;
    font-weight: bold;
  }

  .taskTextOutsideRight.clickable {
    cursor: pointer;
    fill: ${t.taskTextClickableColor} !important;
    font-weight: bold;
  }


  /* Specific task settings for the sections*/

  .taskText0,
  .taskText1,
  .taskText2,
  .taskText3 {
    fill: ${t.taskTextColor};
  }

  .task0,
  .task1,
  .task2,
  .task3 {
    fill: ${t.taskBkgColor};
    stroke: ${t.taskBorderColor};
  }

  .taskTextOutside0,
  .taskTextOutside2
  {
    fill: ${t.taskTextOutsideColor};
  }

  .taskTextOutside1,
  .taskTextOutside3 {
    fill: ${t.taskTextOutsideColor};
  }


  /* Active task */

  .active0,
  .active1,
  .active2,
  .active3 {
    fill: ${t.activeTaskBkgColor};
    stroke: ${t.activeTaskBorderColor};
  }

  .activeText0,
  .activeText1,
  .activeText2,
  .activeText3 {
    fill: ${t.taskTextDarkColor} !important;
  }


  /* Completed task */

  .done0,
  .done1,
  .done2,
  .done3 {
    stroke: ${t.doneTaskBorderColor};
    fill: ${t.doneTaskBkgColor};
    stroke-width: 2;
  }

  .doneText0,
  .doneText1,
  .doneText2,
  .doneText3 {
    fill: ${t.taskTextDarkColor} !important;
  }


  /* Tasks on the critical line */

  .crit0,
  .crit1,
  .crit2,
  .crit3 {
    stroke: ${t.critBorderColor};
    fill: ${t.critBkgColor};
    stroke-width: 2;
  }

  .activeCrit0,
  .activeCrit1,
  .activeCrit2,
  .activeCrit3 {
    stroke: ${t.critBorderColor};
    fill: ${t.activeTaskBkgColor};
    stroke-width: 2;
  }

  .doneCrit0,
  .doneCrit1,
  .doneCrit2,
  .doneCrit3 {
    stroke: ${t.critBorderColor};
    fill: ${t.doneTaskBkgColor};
    stroke-width: 2;
    cursor: pointer;
    shape-rendering: crispEdges;
  }

  .milestone {
    transform: rotate(45deg) scale(0.8,0.8);
  }

  .milestoneText {
    font-style: italic;
  }
  .doneCritText0,
  .doneCritText1,
  .doneCritText2,
  .doneCritText3 {
    fill: ${t.taskTextDarkColor} !important;
  }

  .activeCritText0,
  .activeCritText1,
  .activeCritText2,
  .activeCritText3 {
    fill: ${t.taskTextDarkColor} !important;
  }

  .titleText {
    text-anchor: middle;
    font-size: 18px;
    fill: ${t.titleColor||t.textColor};
    font-family: var(--mermaid-font-family, "trebuchet ms", verdana, arial, sans-serif);
  }
`,Nn=Rn,Xn={parser:$e,db:Wn,renderer:zn,styles:Nn};export{Xn as diagram};
