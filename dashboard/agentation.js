var x1=Object.create;var Vf=Object.defineProperty;var v1=Object.getOwnPropertyDescriptor;var w1=Object.getOwnPropertyNames;var b1=Object.getPrototypeOf,k1=Object.prototype.hasOwnProperty;var Wr=(e,t)=>()=>(t||e((t={exports:{}}).exports,t),t.exports);var C1=(e,t,n,o)=>{if(t&&typeof t=="object"||typeof t=="function")for(let r of w1(t))!k1.call(e,r)&&r!==n&&Vf(e,r,{get:()=>t[r],enumerable:!(o=v1(t,r))||o.enumerable});return e};var xt=(e,t,n)=>(n=e!=null?x1(b1(e)):{},C1(t||!e||!e.__esModule?Vf(n,"default",{value:e,enumerable:!0}):n,e));var ih=Wr(He=>{"use strict";var ml=Symbol.for("react.element"),S1=Symbol.for("react.portal"),M1=Symbol.for("react.fragment"),E1=Symbol.for("react.strict_mode"),L1=Symbol.for("react.profiler"),N1=Symbol.for("react.provider"),I1=Symbol.for("react.context"),R1=Symbol.for("react.forward_ref"),$1=Symbol.for("react.suspense"),T1=Symbol.for("react.memo"),P1=Symbol.for("react.lazy"),Xf=Symbol.iterator;function D1(e){return e===null||typeof e!="object"?null:(e=Xf&&e[Xf]||e["@@iterator"],typeof e=="function"?e:null)}var Kf={isMounted:function(){return!1},enqueueForceUpdate:function(){},enqueueReplaceState:function(){},enqueueSetState:function(){}},Jf=Object.assign,Zf={};function gi(e,t,n){this.props=e,this.context=t,this.refs=Zf,this.updater=n||Kf}gi.prototype.isReactComponent={};gi.prototype.setState=function(e,t){if(typeof e!="object"&&typeof e!="function"&&e!=null)throw Error("setState(...): takes an object of state variables to update or a function which returns an object of state variables.");this.updater.enqueueSetState(this,e,t,"setState")};gi.prototype.forceUpdate=function(e){this.updater.enqueueForceUpdate(this,e,"forceUpdate")};function eh(){}eh.prototype=gi.prototype;function cd(e,t,n){this.props=e,this.context=t,this.refs=Zf,this.updater=n||Kf}var dd=cd.prototype=new eh;dd.constructor=cd;Jf(dd,gi.prototype);dd.isPureReactComponent=!0;var Gf=Array.isArray,th=Object.prototype.hasOwnProperty,ud={current:null},nh={key:!0,ref:!0,__self:!0,__source:!0};function oh(e,t,n){var o,r={},i=null,l=null;if(t!=null)for(o in t.ref!==void 0&&(l=t.ref),t.key!==void 0&&(i=""+t.key),t)th.call(t,o)&&!nh.hasOwnProperty(o)&&(r[o]=t[o]);var s=arguments.length-2;if(s===1)r.children=n;else if(1<s){for(var a=Array(s),c=0;c<s;c++)a[c]=arguments[c+2];r.children=a}if(e&&e.defaultProps)for(o in s=e.defaultProps,s)r[o]===void 0&&(r[o]=s[o]);return{$$typeof:ml,type:e,key:i,ref:l,props:r,_owner:ud.current}}function B1(e,t){return{$$typeof:ml,type:e.type,key:t,ref:e.ref,props:e.props,_owner:e._owner}}function _d(e){return typeof e=="object"&&e!==null&&e.$$typeof===ml}function z1(e){var t={"=":"=0",":":"=2"};return"$"+e.replace(/[=:]/g,function(n){return t[n]})}var qf=/\/+/g;function ad(e,t){return typeof e=="object"&&e!==null&&e.key!=null?z1(""+e.key):t.toString(36)}function Ws(e,t,n,o,r){var i=typeof e;(i==="undefined"||i==="boolean")&&(e=null);var l=!1;if(e===null)l=!0;else switch(i){case"string":case"number":l=!0;break;case"object":switch(e.$$typeof){case ml:case S1:l=!0}}if(l)return l=e,r=r(l),e=o===""?"."+ad(l,0):o,Gf(r)?(n="",e!=null&&(n=e.replace(qf,"$&/")+"/"),Ws(r,t,n,"",function(c){return c})):r!=null&&(_d(r)&&(r=B1(r,n+(!r.key||l&&l.key===r.key?"":(""+r.key).replace(qf,"$&/")+"/")+e)),t.push(r)),1;if(l=0,o=o===""?".":o+":",Gf(e))for(var s=0;s<e.length;s++){i=e[s];var a=o+ad(i,s);l+=Ws(i,t,n,a,r)}else if(a=D1(e),typeof a=="function")for(e=a.call(e),s=0;!(i=e.next()).done;)i=i.value,a=o+ad(i,s++),l+=Ws(i,t,n,a,r);else if(i==="object")throw t=String(e),Error("Objects are not valid as a React child (found: "+(t==="[object Object]"?"object with keys {"+Object.keys(e).join(", ")+"}":t)+"). If you meant to render a collection of children, use an array instead.");return l}function Fs(e,t,n){if(e==null)return e;var o=[],r=0;return Ws(e,o,"","",function(i){return t.call(n,i,r++)}),o}function O1(e){if(e._status===-1){var t=e._result;t=t(),t.then(function(n){(e._status===0||e._status===-1)&&(e._status=1,e._result=n)},function(n){(e._status===0||e._status===-1)&&(e._status=2,e._result=n)}),e._status===-1&&(e._status=0,e._result=t)}if(e._status===1)return e._result.default;throw e._result}var mn={current:null},js={transition:null},A1={ReactCurrentDispatcher:mn,ReactCurrentBatchConfig:js,ReactCurrentOwner:ud};function rh(){throw Error("act(...) is not supported in production builds of React.")}He.Children={map:Fs,forEach:function(e,t,n){Fs(e,function(){t.apply(this,arguments)},n)},count:function(e){var t=0;return Fs(e,function(){t++}),t},toArray:function(e){return Fs(e,function(t){return t})||[]},only:function(e){if(!_d(e))throw Error("React.Children.only expected to receive a single React element child.");return e}};He.Component=gi;He.Fragment=M1;He.Profiler=L1;He.PureComponent=cd;He.StrictMode=E1;He.Suspense=$1;He.__SECRET_INTERNALS_DO_NOT_USE_OR_YOU_WILL_BE_FIRED=A1;He.act=rh;He.cloneElement=function(e,t,n){if(e==null)throw Error("React.cloneElement(...): The argument must be a React element, but you passed "+e+".");var o=Jf({},e.props),r=e.key,i=e.ref,l=e._owner;if(t!=null){if(t.ref!==void 0&&(i=t.ref,l=ud.current),t.key!==void 0&&(r=""+t.key),e.type&&e.type.defaultProps)var s=e.type.defaultProps;for(a in t)th.call(t,a)&&!nh.hasOwnProperty(a)&&(o[a]=t[a]===void 0&&s!==void 0?s[a]:t[a])}var a=arguments.length-2;if(a===1)o.children=n;else if(1<a){s=Array(a);for(var c=0;c<a;c++)s[c]=arguments[c+2];o.children=s}return{$$typeof:ml,type:e.type,key:r,ref:i,props:o,_owner:l}};He.createContext=function(e){return e={$$typeof:I1,_currentValue:e,_currentValue2:e,_threadCount:0,Provider:null,Consumer:null,_defaultValue:null,_globalName:null},e.Provider={$$typeof:N1,_context:e},e.Consumer=e};He.createElement=oh;He.createFactory=function(e){var t=oh.bind(null,e);return t.type=e,t};He.createRef=function(){return{current:null}};He.forwardRef=function(e){return{$$typeof:R1,render:e}};He.isValidElement=_d;He.lazy=function(e){return{$$typeof:P1,_payload:{_status:-1,_result:e},_init:O1}};He.memo=function(e,t){return{$$typeof:T1,type:e,compare:t===void 0?null:t}};He.startTransition=function(e){var t=js.transition;js.transition={};try{e()}finally{js.transition=t}};He.unstable_act=rh;He.useCallback=function(e,t){return mn.current.useCallback(e,t)};He.useContext=function(e){return mn.current.useContext(e)};He.useDebugValue=function(){};He.useDeferredValue=function(e){return mn.current.useDeferredValue(e)};He.useEffect=function(e,t){return mn.current.useEffect(e,t)};He.useId=function(){return mn.current.useId()};He.useImperativeHandle=function(e,t,n){return mn.current.useImperativeHandle(e,t,n)};He.useInsertionEffect=function(e,t){return mn.current.useInsertionEffect(e,t)};He.useLayoutEffect=function(e,t){return mn.current.useLayoutEffect(e,t)};He.useMemo=function(e,t){return mn.current.useMemo(e,t)};He.useReducer=function(e,t,n){return mn.current.useReducer(e,t,n)};He.useRef=function(e){return mn.current.useRef(e)};He.useState=function(e){return mn.current.useState(e)};He.useSyncExternalStore=function(e,t,n){return mn.current.useSyncExternalStore(e,t,n)};He.useTransition=function(){return mn.current.useTransition()};He.version="18.3.1"});var Et=Wr((zw,lh)=>{"use strict";lh.exports=ih()});var mh=Wr(vt=>{"use strict";function md(e,t){var n=e.length;e.push(t);e:for(;0<n;){var o=n-1>>>1,r=e[o];if(0<Hs(r,t))e[o]=t,e[n]=r,n=o;else break e}}function lo(e){return e.length===0?null:e[0]}function Ys(e){if(e.length===0)return null;var t=e[0],n=e.pop();if(n!==t){e[0]=n;e:for(var o=0,r=e.length,i=r>>>1;o<i;){var l=2*(o+1)-1,s=e[l],a=l+1,c=e[a];if(0>Hs(s,n))a<r&&0>Hs(c,s)?(e[o]=c,e[a]=n,o=a):(e[o]=s,e[l]=n,o=l);else if(a<r&&0>Hs(c,n))e[o]=c,e[a]=n,o=a;else break e}}return t}function Hs(e,t){var n=e.sortIndex-t.sortIndex;return n!==0?n:e.id-t.id}typeof performance=="object"&&typeof performance.now=="function"?(sh=performance,vt.unstable_now=function(){return sh.now()}):(fd=Date,ah=fd.now(),vt.unstable_now=function(){return fd.now()-ah});var sh,fd,ah,ko=[],tr=[],F1=1,Yn=null,cn=3,Qs=!1,jr=!1,yl=!1,uh=typeof setTimeout=="function"?setTimeout:null,_h=typeof clearTimeout=="function"?clearTimeout:null,ch=typeof setImmediate<"u"?setImmediate:null;typeof navigator<"u"&&navigator.scheduling!==void 0&&navigator.scheduling.isInputPending!==void 0&&navigator.scheduling.isInputPending.bind(navigator.scheduling);function gd(e){for(var t=lo(tr);t!==null;){if(t.callback===null)Ys(tr);else if(t.startTime<=e)Ys(tr),t.sortIndex=t.expirationTime,md(ko,t);else break;t=lo(tr)}}function yd(e){if(yl=!1,gd(e),!jr)if(lo(ko)!==null)jr=!0,vd(xd);else{var t=lo(tr);t!==null&&wd(yd,t.startTime-e)}}function xd(e,t){jr=!1,yl&&(yl=!1,_h(xl),xl=-1),Qs=!0;var n=cn;try{for(gd(t),Yn=lo(ko);Yn!==null&&(!(Yn.expirationTime>t)||e&&!ph());){var o=Yn.callback;if(typeof o=="function"){Yn.callback=null,cn=Yn.priorityLevel;var r=o(Yn.expirationTime<=t);t=vt.unstable_now(),typeof r=="function"?Yn.callback=r:Yn===lo(ko)&&Ys(ko),gd(t)}else Ys(ko);Yn=lo(ko)}if(Yn!==null)var i=!0;else{var l=lo(tr);l!==null&&wd(yd,l.startTime-t),i=!1}return i}finally{Yn=null,cn=n,Qs=!1}}var Vs=!1,Us=null,xl=-1,fh=5,hh=-1;function ph(){return!(vt.unstable_now()-hh<fh)}function hd(){if(Us!==null){var e=vt.unstable_now();hh=e;var t=!0;try{t=Us(!0,e)}finally{t?gl():(Vs=!1,Us=null)}}else Vs=!1}var gl;typeof ch=="function"?gl=function(){ch(hd)}:typeof MessageChannel<"u"?(pd=new MessageChannel,dh=pd.port2,pd.port1.onmessage=hd,gl=function(){dh.postMessage(null)}):gl=function(){uh(hd,0)};var pd,dh;function vd(e){Us=e,Vs||(Vs=!0,gl())}function wd(e,t){xl=uh(function(){e(vt.unstable_now())},t)}vt.unstable_IdlePriority=5;vt.unstable_ImmediatePriority=1;vt.unstable_LowPriority=4;vt.unstable_NormalPriority=3;vt.unstable_Profiling=null;vt.unstable_UserBlockingPriority=2;vt.unstable_cancelCallback=function(e){e.callback=null};vt.unstable_continueExecution=function(){jr||Qs||(jr=!0,vd(xd))};vt.unstable_forceFrameRate=function(e){0>e||125<e?console.error("forceFrameRate takes a positive int between 0 and 125, forcing frame rates higher than 125 fps is not supported"):fh=0<e?Math.floor(1e3/e):5};vt.unstable_getCurrentPriorityLevel=function(){return cn};vt.unstable_getFirstCallbackNode=function(){return lo(ko)};vt.unstable_next=function(e){switch(cn){case 1:case 2:case 3:var t=3;break;default:t=cn}var n=cn;cn=t;try{return e()}finally{cn=n}};vt.unstable_pauseExecution=function(){};vt.unstable_requestPaint=function(){};vt.unstable_runWithPriority=function(e,t){switch(e){case 1:case 2:case 3:case 4:case 5:break;default:e=3}var n=cn;cn=e;try{return t()}finally{cn=n}};vt.unstable_scheduleCallback=function(e,t,n){var o=vt.unstable_now();switch(typeof n=="object"&&n!==null?(n=n.delay,n=typeof n=="number"&&0<n?o+n:o):n=o,e){case 1:var r=-1;break;case 2:r=250;break;case 5:r=1073741823;break;case 4:r=1e4;break;default:r=5e3}return r=n+r,e={id:F1++,callback:t,priorityLevel:e,startTime:n,expirationTime:r,sortIndex:-1},n>o?(e.sortIndex=n,md(tr,e),lo(ko)===null&&e===lo(tr)&&(yl?(_h(xl),xl=-1):yl=!0,wd(yd,n-o))):(e.sortIndex=r,md(ko,e),jr||Qs||(jr=!0,vd(xd))),e};vt.unstable_shouldYield=ph;vt.unstable_wrapCallback=function(e){var t=cn;return function(){var n=cn;cn=t;try{return e.apply(this,arguments)}finally{cn=n}}}});var yh=Wr((Aw,gh)=>{"use strict";gh.exports=mh()});var b0=Wr(Bn=>{"use strict";var W1=Et(),Pn=yh();function Q(e){for(var t="https://reactjs.org/docs/error-decoder.html?invariant="+e,n=1;n<arguments.length;n++)t+="&args[]="+encodeURIComponent(arguments[n]);return"Minified React error #"+e+"; visit "+t+" for the full message or use the non-minified dev environment for full errors and additional helpful warnings."}var Sp=new Set,Wl={};function ti(e,t){Oi(e,t),Oi(e+"Capture",t)}function Oi(e,t){for(Wl[e]=t,e=0;e<t.length;e++)Sp.add(t[e])}var Yo=!(typeof window>"u"||typeof window.document>"u"||typeof window.document.createElement>"u"),Ud=Object.prototype.hasOwnProperty,j1=/^[:A-Z_a-z\u00C0-\u00D6\u00D8-\u00F6\u00F8-\u02FF\u0370-\u037D\u037F-\u1FFF\u200C-\u200D\u2070-\u218F\u2C00-\u2FEF\u3001-\uD7FF\uF900-\uFDCF\uFDF0-\uFFFD][:A-Z_a-z\u00C0-\u00D6\u00D8-\u00F6\u00F8-\u02FF\u0370-\u037D\u037F-\u1FFF\u200C-\u200D\u2070-\u218F\u2C00-\u2FEF\u3001-\uD7FF\uF900-\uFDCF\uFDF0-\uFFFD\-.0-9\u00B7\u0300-\u036F\u203F-\u2040]*$/,xh={},vh={};function H1(e){return Ud.call(vh,e)?!0:Ud.call(xh,e)?!1:j1.test(e)?vh[e]=!0:(xh[e]=!0,!1)}function U1(e,t,n,o){if(n!==null&&n.type===0)return!1;switch(typeof t){case"function":case"symbol":return!0;case"boolean":return o?!1:n!==null?!n.acceptsBooleans:(e=e.toLowerCase().slice(0,5),e!=="data-"&&e!=="aria-");default:return!1}}function Y1(e,t,n,o){if(t===null||typeof t>"u"||U1(e,t,n,o))return!0;if(o)return!1;if(n!==null)switch(n.type){case 3:return!t;case 4:return t===!1;case 5:return isNaN(t);case 6:return isNaN(t)||1>t}return!1}function xn(e,t,n,o,r,i,l){this.acceptsBooleans=t===2||t===3||t===4,this.attributeName=o,this.attributeNamespace=r,this.mustUseProperty=n,this.propertyName=e,this.type=t,this.sanitizeURL=i,this.removeEmptyString=l}var en={};"children dangerouslySetInnerHTML defaultValue defaultChecked innerHTML suppressContentEditableWarning suppressHydrationWarning style".split(" ").forEach(function(e){en[e]=new xn(e,0,!1,e,null,!1,!1)});[["acceptCharset","accept-charset"],["className","class"],["htmlFor","for"],["httpEquiv","http-equiv"]].forEach(function(e){var t=e[0];en[t]=new xn(t,1,!1,e[1],null,!1,!1)});["contentEditable","draggable","spellCheck","value"].forEach(function(e){en[e]=new xn(e,2,!1,e.toLowerCase(),null,!1,!1)});["autoReverse","externalResourcesRequired","focusable","preserveAlpha"].forEach(function(e){en[e]=new xn(e,2,!1,e,null,!1,!1)});"allowFullScreen async autoFocus autoPlay controls default defer disabled disablePictureInPicture disableRemotePlayback formNoValidate hidden loop noModule noValidate open playsInline readOnly required reversed scoped seamless itemScope".split(" ").forEach(function(e){en[e]=new xn(e,3,!1,e.toLowerCase(),null,!1,!1)});["checked","multiple","muted","selected"].forEach(function(e){en[e]=new xn(e,3,!0,e,null,!1,!1)});["capture","download"].forEach(function(e){en[e]=new xn(e,4,!1,e,null,!1,!1)});["cols","rows","size","span"].forEach(function(e){en[e]=new xn(e,6,!1,e,null,!1,!1)});["rowSpan","start"].forEach(function(e){en[e]=new xn(e,5,!1,e.toLowerCase(),null,!1,!1)});var Bu=/[\-:]([a-z])/g;function zu(e){return e[1].toUpperCase()}"accent-height alignment-baseline arabic-form baseline-shift cap-height clip-path clip-rule color-interpolation color-interpolation-filters color-profile color-rendering dominant-baseline enable-background fill-opacity fill-rule flood-color flood-opacity font-family font-size font-size-adjust font-stretch font-style font-variant font-weight glyph-name glyph-orientation-horizontal glyph-orientation-vertical horiz-adv-x horiz-origin-x image-rendering letter-spacing lighting-color marker-end marker-mid marker-start overline-position overline-thickness paint-order panose-1 pointer-events rendering-intent shape-rendering stop-color stop-opacity strikethrough-position strikethrough-thickness stroke-dasharray stroke-dashoffset stroke-linecap stroke-linejoin stroke-miterlimit stroke-opacity stroke-width text-anchor text-decoration text-rendering underline-position underline-thickness unicode-bidi unicode-range units-per-em v-alphabetic v-hanging v-ideographic v-mathematical vector-effect vert-adv-y vert-origin-x vert-origin-y word-spacing writing-mode xmlns:xlink x-height".split(" ").forEach(function(e){var t=e.replace(Bu,zu);en[t]=new xn(t,1,!1,e,null,!1,!1)});"xlink:actuate xlink:arcrole xlink:role xlink:show xlink:title xlink:type".split(" ").forEach(function(e){var t=e.replace(Bu,zu);en[t]=new xn(t,1,!1,e,"http://www.w3.org/1999/xlink",!1,!1)});["xml:base","xml:lang","xml:space"].forEach(function(e){var t=e.replace(Bu,zu);en[t]=new xn(t,1,!1,e,"http://www.w3.org/XML/1998/namespace",!1,!1)});["tabIndex","crossOrigin"].forEach(function(e){en[e]=new xn(e,1,!1,e.toLowerCase(),null,!1,!1)});en.xlinkHref=new xn("xlinkHref",1,!1,"xlink:href","http://www.w3.org/1999/xlink",!0,!1);["src","href","action","formAction"].forEach(function(e){en[e]=new xn(e,1,!1,e.toLowerCase(),null,!0,!0)});function Ou(e,t,n,o){var r=en.hasOwnProperty(t)?en[t]:null;(r!==null?r.type!==0:o||!(2<t.length)||t[0]!=="o"&&t[0]!=="O"||t[1]!=="n"&&t[1]!=="N")&&(Y1(t,n,r,o)&&(n=null),o||r===null?H1(t)&&(n===null?e.removeAttribute(t):e.setAttribute(t,""+n)):r.mustUseProperty?e[r.propertyName]=n===null?r.type===3?!1:"":n:(t=r.attributeName,o=r.attributeNamespace,n===null?e.removeAttribute(t):(r=r.type,n=r===3||r===4&&n===!0?"":""+n,o?e.setAttributeNS(o,t,n):e.setAttribute(t,n))))}var Go=W1.__SECRET_INTERNALS_DO_NOT_USE_OR_YOU_WILL_BE_FIRED,Xs=Symbol.for("react.element"),vi=Symbol.for("react.portal"),wi=Symbol.for("react.fragment"),Au=Symbol.for("react.strict_mode"),Yd=Symbol.for("react.profiler"),Mp=Symbol.for("react.provider"),Ep=Symbol.for("react.context"),Fu=Symbol.for("react.forward_ref"),Qd=Symbol.for("react.suspense"),Vd=Symbol.for("react.suspense_list"),Wu=Symbol.for("react.memo"),or=Symbol.for("react.lazy"),Lp=Symbol.for("react.offscreen"),wh=Symbol.iterator;function vl(e){return e===null||typeof e!="object"?null:(e=wh&&e[wh]||e["@@iterator"],typeof e=="function"?e:null)}var Rt=Object.assign,bd;function Ll(e){if(bd===void 0)try{throw Error()}catch(n){var t=n.stack.trim().match(/\n( *(at )?)/);bd=t&&t[1]||""}return`
`+bd+e}var kd=!1;function Cd(e,t){if(!e||kd)return"";kd=!0;var n=Error.prepareStackTrace;Error.prepareStackTrace=void 0;try{if(t)if(t=function(){throw Error()},Object.defineProperty(t.prototype,"props",{set:function(){throw Error()}}),typeof Reflect=="object"&&Reflect.construct){try{Reflect.construct(t,[])}catch(c){var o=c}Reflect.construct(e,[],t)}else{try{t.call()}catch(c){o=c}e.call(t.prototype)}else{try{throw Error()}catch(c){o=c}e()}}catch(c){if(c&&o&&typeof c.stack=="string"){for(var r=c.stack.split(`
`),i=o.stack.split(`
`),l=r.length-1,s=i.length-1;1<=l&&0<=s&&r[l]!==i[s];)s--;for(;1<=l&&0<=s;l--,s--)if(r[l]!==i[s]){if(l!==1||s!==1)do if(l--,s--,0>s||r[l]!==i[s]){var a=`
`+r[l].replace(" at new "," at ");return e.displayName&&a.includes("<anonymous>")&&(a=a.replace("<anonymous>",e.displayName)),a}while(1<=l&&0<=s);break}}}finally{kd=!1,Error.prepareStackTrace=n}return(e=e?e.displayName||e.name:"")?Ll(e):""}function Q1(e){switch(e.tag){case 5:return Ll(e.type);case 16:return Ll("Lazy");case 13:return Ll("Suspense");case 19:return Ll("SuspenseList");case 0:case 2:case 15:return e=Cd(e.type,!1),e;case 11:return e=Cd(e.type.render,!1),e;case 1:return e=Cd(e.type,!0),e;default:return""}}function Xd(e){if(e==null)return null;if(typeof e=="function")return e.displayName||e.name||null;if(typeof e=="string")return e;switch(e){case wi:return"Fragment";case vi:return"Portal";case Yd:return"Profiler";case Au:return"StrictMode";case Qd:return"Suspense";case Vd:return"SuspenseList"}if(typeof e=="object")switch(e.$$typeof){case Ep:return(e.displayName||"Context")+".Consumer";case Mp:return(e._context.displayName||"Context")+".Provider";case Fu:var t=e.render;return e=e.displayName,e||(e=t.displayName||t.name||"",e=e!==""?"ForwardRef("+e+")":"ForwardRef"),e;case Wu:return t=e.displayName||null,t!==null?t:Xd(e.type)||"Memo";case or:t=e._payload,e=e._init;try{return Xd(e(t))}catch{}}return null}function V1(e){var t=e.type;switch(e.tag){case 24:return"Cache";case 9:return(t.displayName||"Context")+".Consumer";case 10:return(t._context.displayName||"Context")+".Provider";case 18:return"DehydratedFragment";case 11:return e=t.render,e=e.displayName||e.name||"",t.displayName||(e!==""?"ForwardRef("+e+")":"ForwardRef");case 7:return"Fragment";case 5:return t;case 4:return"Portal";case 3:return"Root";case 6:return"Text";case 16:return Xd(t);case 8:return t===Au?"StrictMode":"Mode";case 22:return"Offscreen";case 12:return"Profiler";case 21:return"Scope";case 13:return"Suspense";case 19:return"SuspenseList";case 25:return"TracingMarker";case 1:case 0:case 17:case 2:case 14:case 15:if(typeof t=="function")return t.displayName||t.name||null;if(typeof t=="string")return t}return null}function gr(e){switch(typeof e){case"boolean":case"number":case"string":case"undefined":return e;case"object":return e;default:return""}}function Np(e){var t=e.type;return(e=e.nodeName)&&e.toLowerCase()==="input"&&(t==="checkbox"||t==="radio")}function X1(e){var t=Np(e)?"checked":"value",n=Object.getOwnPropertyDescriptor(e.constructor.prototype,t),o=""+e[t];if(!e.hasOwnProperty(t)&&typeof n<"u"&&typeof n.get=="function"&&typeof n.set=="function"){var r=n.get,i=n.set;return Object.defineProperty(e,t,{configurable:!0,get:function(){return r.call(this)},set:function(l){o=""+l,i.call(this,l)}}),Object.defineProperty(e,t,{enumerable:n.enumerable}),{getValue:function(){return o},setValue:function(l){o=""+l},stopTracking:function(){e._valueTracker=null,delete e[t]}}}}function Gs(e){e._valueTracker||(e._valueTracker=X1(e))}function Ip(e){if(!e)return!1;var t=e._valueTracker;if(!t)return!0;var n=t.getValue(),o="";return e&&(o=Np(e)?e.checked?"true":"false":e.value),e=o,e!==n?(t.setValue(e),!0):!1}function ka(e){if(e=e||(typeof document<"u"?document:void 0),typeof e>"u")return null;try{return e.activeElement||e.body}catch{return e.body}}function Gd(e,t){var n=t.checked;return Rt({},t,{defaultChecked:void 0,defaultValue:void 0,value:void 0,checked:n??e._wrapperState.initialChecked})}function bh(e,t){var n=t.defaultValue==null?"":t.defaultValue,o=t.checked!=null?t.checked:t.defaultChecked;n=gr(t.value!=null?t.value:n),e._wrapperState={initialChecked:o,initialValue:n,controlled:t.type==="checkbox"||t.type==="radio"?t.checked!=null:t.value!=null}}function Rp(e,t){t=t.checked,t!=null&&Ou(e,"checked",t,!1)}function qd(e,t){Rp(e,t);var n=gr(t.value),o=t.type;if(n!=null)o==="number"?(n===0&&e.value===""||e.value!=n)&&(e.value=""+n):e.value!==""+n&&(e.value=""+n);else if(o==="submit"||o==="reset"){e.removeAttribute("value");return}t.hasOwnProperty("value")?Kd(e,t.type,n):t.hasOwnProperty("defaultValue")&&Kd(e,t.type,gr(t.defaultValue)),t.checked==null&&t.defaultChecked!=null&&(e.defaultChecked=!!t.defaultChecked)}function kh(e,t,n){if(t.hasOwnProperty("value")||t.hasOwnProperty("defaultValue")){var o=t.type;if(!(o!=="submit"&&o!=="reset"||t.value!==void 0&&t.value!==null))return;t=""+e._wrapperState.initialValue,n||t===e.value||(e.value=t),e.defaultValue=t}n=e.name,n!==""&&(e.name=""),e.defaultChecked=!!e._wrapperState.initialChecked,n!==""&&(e.name=n)}function Kd(e,t,n){(t!=="number"||ka(e.ownerDocument)!==e)&&(n==null?e.defaultValue=""+e._wrapperState.initialValue:e.defaultValue!==""+n&&(e.defaultValue=""+n))}var Nl=Array.isArray;function $i(e,t,n,o){if(e=e.options,t){t={};for(var r=0;r<n.length;r++)t["$"+n[r]]=!0;for(n=0;n<e.length;n++)r=t.hasOwnProperty("$"+e[n].value),e[n].selected!==r&&(e[n].selected=r),r&&o&&(e[n].defaultSelected=!0)}else{for(n=""+gr(n),t=null,r=0;r<e.length;r++){if(e[r].value===n){e[r].selected=!0,o&&(e[r].defaultSelected=!0);return}t!==null||e[r].disabled||(t=e[r])}t!==null&&(t.selected=!0)}}function Jd(e,t){if(t.dangerouslySetInnerHTML!=null)throw Error(Q(91));return Rt({},t,{value:void 0,defaultValue:void 0,children:""+e._wrapperState.initialValue})}function Ch(e,t){var n=t.value;if(n==null){if(n=t.children,t=t.defaultValue,n!=null){if(t!=null)throw Error(Q(92));if(Nl(n)){if(1<n.length)throw Error(Q(93));n=n[0]}t=n}t==null&&(t=""),n=t}e._wrapperState={initialValue:gr(n)}}function $p(e,t){var n=gr(t.value),o=gr(t.defaultValue);n!=null&&(n=""+n,n!==e.value&&(e.value=n),t.defaultValue==null&&e.defaultValue!==n&&(e.defaultValue=n)),o!=null&&(e.defaultValue=""+o)}function Sh(e){var t=e.textContent;t===e._wrapperState.initialValue&&t!==""&&t!==null&&(e.value=t)}function Tp(e){switch(e){case"svg":return"http://www.w3.org/2000/svg";case"math":return"http://www.w3.org/1998/Math/MathML";default:return"http://www.w3.org/1999/xhtml"}}function Zd(e,t){return e==null||e==="http://www.w3.org/1999/xhtml"?Tp(t):e==="http://www.w3.org/2000/svg"&&t==="foreignObject"?"http://www.w3.org/1999/xhtml":e}var qs,Pp=(function(e){return typeof MSApp<"u"&&MSApp.execUnsafeLocalFunction?function(t,n,o,r){MSApp.execUnsafeLocalFunction(function(){return e(t,n,o,r)})}:e})(function(e,t){if(e.namespaceURI!=="http://www.w3.org/2000/svg"||"innerHTML"in e)e.innerHTML=t;else{for(qs=qs||document.createElement("div"),qs.innerHTML="<svg>"+t.valueOf().toString()+"</svg>",t=qs.firstChild;e.firstChild;)e.removeChild(e.firstChild);for(;t.firstChild;)e.appendChild(t.firstChild)}});function jl(e,t){if(t){var n=e.firstChild;if(n&&n===e.lastChild&&n.nodeType===3){n.nodeValue=t;return}}e.textContent=t}var $l={animationIterationCount:!0,aspectRatio:!0,borderImageOutset:!0,borderImageSlice:!0,borderImageWidth:!0,boxFlex:!0,boxFlexGroup:!0,boxOrdinalGroup:!0,columnCount:!0,columns:!0,flex:!0,flexGrow:!0,flexPositive:!0,flexShrink:!0,flexNegative:!0,flexOrder:!0,gridArea:!0,gridRow:!0,gridRowEnd:!0,gridRowSpan:!0,gridRowStart:!0,gridColumn:!0,gridColumnEnd:!0,gridColumnSpan:!0,gridColumnStart:!0,fontWeight:!0,lineClamp:!0,lineHeight:!0,opacity:!0,order:!0,orphans:!0,tabSize:!0,widows:!0,zIndex:!0,zoom:!0,fillOpacity:!0,floodOpacity:!0,stopOpacity:!0,strokeDasharray:!0,strokeDashoffset:!0,strokeMiterlimit:!0,strokeOpacity:!0,strokeWidth:!0},G1=["Webkit","ms","Moz","O"];Object.keys($l).forEach(function(e){G1.forEach(function(t){t=t+e.charAt(0).toUpperCase()+e.substring(1),$l[t]=$l[e]})});function Dp(e,t,n){return t==null||typeof t=="boolean"||t===""?"":n||typeof t!="number"||t===0||$l.hasOwnProperty(e)&&$l[e]?(""+t).trim():t+"px"}function Bp(e,t){e=e.style;for(var n in t)if(t.hasOwnProperty(n)){var o=n.indexOf("--")===0,r=Dp(n,t[n],o);n==="float"&&(n="cssFloat"),o?e.setProperty(n,r):e[n]=r}}var q1=Rt({menuitem:!0},{area:!0,base:!0,br:!0,col:!0,embed:!0,hr:!0,img:!0,input:!0,keygen:!0,link:!0,meta:!0,param:!0,source:!0,track:!0,wbr:!0});function eu(e,t){if(t){if(q1[e]&&(t.children!=null||t.dangerouslySetInnerHTML!=null))throw Error(Q(137,e));if(t.dangerouslySetInnerHTML!=null){if(t.children!=null)throw Error(Q(60));if(typeof t.dangerouslySetInnerHTML!="object"||!("__html"in t.dangerouslySetInnerHTML))throw Error(Q(61))}if(t.style!=null&&typeof t.style!="object")throw Error(Q(62))}}function tu(e,t){if(e.indexOf("-")===-1)return typeof t.is=="string";switch(e){case"annotation-xml":case"color-profile":case"font-face":case"font-face-src":case"font-face-uri":case"font-face-format":case"font-face-name":case"missing-glyph":return!1;default:return!0}}var nu=null;function ju(e){return e=e.target||e.srcElement||window,e.correspondingUseElement&&(e=e.correspondingUseElement),e.nodeType===3?e.parentNode:e}var ou=null,Ti=null,Pi=null;function Mh(e){if(e=ls(e)){if(typeof ou!="function")throw Error(Q(280));var t=e.stateNode;t&&(t=Ka(t),ou(e.stateNode,e.type,t))}}function zp(e){Ti?Pi?Pi.push(e):Pi=[e]:Ti=e}function Op(){if(Ti){var e=Ti,t=Pi;if(Pi=Ti=null,Mh(e),t)for(e=0;e<t.length;e++)Mh(t[e])}}function Ap(e,t){return e(t)}function Fp(){}var Sd=!1;function Wp(e,t,n){if(Sd)return e(t,n);Sd=!0;try{return Ap(e,t,n)}finally{Sd=!1,(Ti!==null||Pi!==null)&&(Fp(),Op())}}function Hl(e,t){var n=e.stateNode;if(n===null)return null;var o=Ka(n);if(o===null)return null;n=o[t];e:switch(t){case"onClick":case"onClickCapture":case"onDoubleClick":case"onDoubleClickCapture":case"onMouseDown":case"onMouseDownCapture":case"onMouseMove":case"onMouseMoveCapture":case"onMouseUp":case"onMouseUpCapture":case"onMouseEnter":(o=!o.disabled)||(e=e.type,o=!(e==="button"||e==="input"||e==="select"||e==="textarea")),e=!o;break e;default:e=!1}if(e)return null;if(n&&typeof n!="function")throw Error(Q(231,t,typeof n));return n}var ru=!1;if(Yo)try{yi={},Object.defineProperty(yi,"passive",{get:function(){ru=!0}}),window.addEventListener("test",yi,yi),window.removeEventListener("test",yi,yi)}catch{ru=!1}var yi;function K1(e,t,n,o,r,i,l,s,a){var c=Array.prototype.slice.call(arguments,3);try{t.apply(n,c)}catch(f){this.onError(f)}}var Tl=!1,Ca=null,Sa=!1,iu=null,J1={onError:function(e){Tl=!0,Ca=e}};function Z1(e,t,n,o,r,i,l,s,a){Tl=!1,Ca=null,K1.apply(J1,arguments)}function ey(e,t,n,o,r,i,l,s,a){if(Z1.apply(this,arguments),Tl){if(Tl){var c=Ca;Tl=!1,Ca=null}else throw Error(Q(198));Sa||(Sa=!0,iu=c)}}function ni(e){var t=e,n=e;if(e.alternate)for(;t.return;)t=t.return;else{e=t;do t=e,(t.flags&4098)!==0&&(n=t.return),e=t.return;while(e)}return t.tag===3?n:null}function jp(e){if(e.tag===13){var t=e.memoizedState;if(t===null&&(e=e.alternate,e!==null&&(t=e.memoizedState)),t!==null)return t.dehydrated}return null}function Eh(e){if(ni(e)!==e)throw Error(Q(188))}function ty(e){var t=e.alternate;if(!t){if(t=ni(e),t===null)throw Error(Q(188));return t!==e?null:e}for(var n=e,o=t;;){var r=n.return;if(r===null)break;var i=r.alternate;if(i===null){if(o=r.return,o!==null){n=o;continue}break}if(r.child===i.child){for(i=r.child;i;){if(i===n)return Eh(r),e;if(i===o)return Eh(r),t;i=i.sibling}throw Error(Q(188))}if(n.return!==o.return)n=r,o=i;else{for(var l=!1,s=r.child;s;){if(s===n){l=!0,n=r,o=i;break}if(s===o){l=!0,o=r,n=i;break}s=s.sibling}if(!l){for(s=i.child;s;){if(s===n){l=!0,n=i,o=r;break}if(s===o){l=!0,o=i,n=r;break}s=s.sibling}if(!l)throw Error(Q(189))}}if(n.alternate!==o)throw Error(Q(190))}if(n.tag!==3)throw Error(Q(188));return n.stateNode.current===n?e:t}function Hp(e){return e=ty(e),e!==null?Up(e):null}function Up(e){if(e.tag===5||e.tag===6)return e;for(e=e.child;e!==null;){var t=Up(e);if(t!==null)return t;e=e.sibling}return null}var Yp=Pn.unstable_scheduleCallback,Lh=Pn.unstable_cancelCallback,ny=Pn.unstable_shouldYield,oy=Pn.unstable_requestPaint,zt=Pn.unstable_now,ry=Pn.unstable_getCurrentPriorityLevel,Hu=Pn.unstable_ImmediatePriority,Qp=Pn.unstable_UserBlockingPriority,Ma=Pn.unstable_NormalPriority,iy=Pn.unstable_LowPriority,Vp=Pn.unstable_IdlePriority,Va=null,Eo=null;function ly(e){if(Eo&&typeof Eo.onCommitFiberRoot=="function")try{Eo.onCommitFiberRoot(Va,e,void 0,(e.current.flags&128)===128)}catch{}}var _o=Math.clz32?Math.clz32:cy,sy=Math.log,ay=Math.LN2;function cy(e){return e>>>=0,e===0?32:31-(sy(e)/ay|0)|0}var Ks=64,Js=4194304;function Il(e){switch(e&-e){case 1:return 1;case 2:return 2;case 4:return 4;case 8:return 8;case 16:return 16;case 32:return 32;case 64:case 128:case 256:case 512:case 1024:case 2048:case 4096:case 8192:case 16384:case 32768:case 65536:case 131072:case 262144:case 524288:case 1048576:case 2097152:return e&4194240;case 4194304:case 8388608:case 16777216:case 33554432:case 67108864:return e&130023424;case 134217728:return 134217728;case 268435456:return 268435456;case 536870912:return 536870912;case 1073741824:return 1073741824;default:return e}}function Ea(e,t){var n=e.pendingLanes;if(n===0)return 0;var o=0,r=e.suspendedLanes,i=e.pingedLanes,l=n&268435455;if(l!==0){var s=l&~r;s!==0?o=Il(s):(i&=l,i!==0&&(o=Il(i)))}else l=n&~r,l!==0?o=Il(l):i!==0&&(o=Il(i));if(o===0)return 0;if(t!==0&&t!==o&&(t&r)===0&&(r=o&-o,i=t&-t,r>=i||r===16&&(i&4194240)!==0))return t;if((o&4)!==0&&(o|=n&16),t=e.entangledLanes,t!==0)for(e=e.entanglements,t&=o;0<t;)n=31-_o(t),r=1<<n,o|=e[n],t&=~r;return o}function dy(e,t){switch(e){case 1:case 2:case 4:return t+250;case 8:case 16:case 32:case 64:case 128:case 256:case 512:case 1024:case 2048:case 4096:case 8192:case 16384:case 32768:case 65536:case 131072:case 262144:case 524288:case 1048576:case 2097152:return t+5e3;case 4194304:case 8388608:case 16777216:case 33554432:case 67108864:return-1;case 134217728:case 268435456:case 536870912:case 1073741824:return-1;default:return-1}}function uy(e,t){for(var n=e.suspendedLanes,o=e.pingedLanes,r=e.expirationTimes,i=e.pendingLanes;0<i;){var l=31-_o(i),s=1<<l,a=r[l];a===-1?((s&n)===0||(s&o)!==0)&&(r[l]=dy(s,t)):a<=t&&(e.expiredLanes|=s),i&=~s}}function lu(e){return e=e.pendingLanes&-1073741825,e!==0?e:e&1073741824?1073741824:0}function Xp(){var e=Ks;return Ks<<=1,(Ks&4194240)===0&&(Ks=64),e}function Md(e){for(var t=[],n=0;31>n;n++)t.push(e);return t}function rs(e,t,n){e.pendingLanes|=t,t!==536870912&&(e.suspendedLanes=0,e.pingedLanes=0),e=e.eventTimes,t=31-_o(t),e[t]=n}function _y(e,t){var n=e.pendingLanes&~t;e.pendingLanes=t,e.suspendedLanes=0,e.pingedLanes=0,e.expiredLanes&=t,e.mutableReadLanes&=t,e.entangledLanes&=t,t=e.entanglements;var o=e.eventTimes;for(e=e.expirationTimes;0<n;){var r=31-_o(n),i=1<<r;t[r]=0,o[r]=-1,e[r]=-1,n&=~i}}function Uu(e,t){var n=e.entangledLanes|=t;for(e=e.entanglements;n;){var o=31-_o(n),r=1<<o;r&t|e[o]&t&&(e[o]|=t),n&=~r}}var dt=0;function Gp(e){return e&=-e,1<e?4<e?(e&268435455)!==0?16:536870912:4:1}var qp,Yu,Kp,Jp,Zp,su=!1,Zs=[],cr=null,dr=null,ur=null,Ul=new Map,Yl=new Map,ir=[],fy="mousedown mouseup touchcancel touchend touchstart auxclick dblclick pointercancel pointerdown pointerup dragend dragstart drop compositionend compositionstart keydown keypress keyup input textInput copy cut paste click change contextmenu reset submit".split(" ");function Nh(e,t){switch(e){case"focusin":case"focusout":cr=null;break;case"dragenter":case"dragleave":dr=null;break;case"mouseover":case"mouseout":ur=null;break;case"pointerover":case"pointerout":Ul.delete(t.pointerId);break;case"gotpointercapture":case"lostpointercapture":Yl.delete(t.pointerId)}}function wl(e,t,n,o,r,i){return e===null||e.nativeEvent!==i?(e={blockedOn:t,domEventName:n,eventSystemFlags:o,nativeEvent:i,targetContainers:[r]},t!==null&&(t=ls(t),t!==null&&Yu(t)),e):(e.eventSystemFlags|=o,t=e.targetContainers,r!==null&&t.indexOf(r)===-1&&t.push(r),e)}function hy(e,t,n,o,r){switch(t){case"focusin":return cr=wl(cr,e,t,n,o,r),!0;case"dragenter":return dr=wl(dr,e,t,n,o,r),!0;case"mouseover":return ur=wl(ur,e,t,n,o,r),!0;case"pointerover":var i=r.pointerId;return Ul.set(i,wl(Ul.get(i)||null,e,t,n,o,r)),!0;case"gotpointercapture":return i=r.pointerId,Yl.set(i,wl(Yl.get(i)||null,e,t,n,o,r)),!0}return!1}function em(e){var t=Yr(e.target);if(t!==null){var n=ni(t);if(n!==null){if(t=n.tag,t===13){if(t=jp(n),t!==null){e.blockedOn=t,Zp(e.priority,function(){Kp(n)});return}}else if(t===3&&n.stateNode.current.memoizedState.isDehydrated){e.blockedOn=n.tag===3?n.stateNode.containerInfo:null;return}}}e.blockedOn=null}function fa(e){if(e.blockedOn!==null)return!1;for(var t=e.targetContainers;0<t.length;){var n=au(e.domEventName,e.eventSystemFlags,t[0],e.nativeEvent);if(n===null){n=e.nativeEvent;var o=new n.constructor(n.type,n);nu=o,n.target.dispatchEvent(o),nu=null}else return t=ls(n),t!==null&&Yu(t),e.blockedOn=n,!1;t.shift()}return!0}function Ih(e,t,n){fa(e)&&n.delete(t)}function py(){su=!1,cr!==null&&fa(cr)&&(cr=null),dr!==null&&fa(dr)&&(dr=null),ur!==null&&fa(ur)&&(ur=null),Ul.forEach(Ih),Yl.forEach(Ih)}function bl(e,t){e.blockedOn===t&&(e.blockedOn=null,su||(su=!0,Pn.unstable_scheduleCallback(Pn.unstable_NormalPriority,py)))}function Ql(e){function t(r){return bl(r,e)}if(0<Zs.length){bl(Zs[0],e);for(var n=1;n<Zs.length;n++){var o=Zs[n];o.blockedOn===e&&(o.blockedOn=null)}}for(cr!==null&&bl(cr,e),dr!==null&&bl(dr,e),ur!==null&&bl(ur,e),Ul.forEach(t),Yl.forEach(t),n=0;n<ir.length;n++)o=ir[n],o.blockedOn===e&&(o.blockedOn=null);for(;0<ir.length&&(n=ir[0],n.blockedOn===null);)em(n),n.blockedOn===null&&ir.shift()}var Di=Go.ReactCurrentBatchConfig,La=!0;function my(e,t,n,o){var r=dt,i=Di.transition;Di.transition=null;try{dt=1,Qu(e,t,n,o)}finally{dt=r,Di.transition=i}}function gy(e,t,n,o){var r=dt,i=Di.transition;Di.transition=null;try{dt=4,Qu(e,t,n,o)}finally{dt=r,Di.transition=i}}function Qu(e,t,n,o){if(La){var r=au(e,t,n,o);if(r===null)Td(e,t,o,Na,n),Nh(e,o);else if(hy(r,e,t,n,o))o.stopPropagation();else if(Nh(e,o),t&4&&-1<fy.indexOf(e)){for(;r!==null;){var i=ls(r);if(i!==null&&qp(i),i=au(e,t,n,o),i===null&&Td(e,t,o,Na,n),i===r)break;r=i}r!==null&&o.stopPropagation()}else Td(e,t,o,null,n)}}var Na=null;function au(e,t,n,o){if(Na=null,e=ju(o),e=Yr(e),e!==null)if(t=ni(e),t===null)e=null;else if(n=t.tag,n===13){if(e=jp(t),e!==null)return e;e=null}else if(n===3){if(t.stateNode.current.memoizedState.isDehydrated)return t.tag===3?t.stateNode.containerInfo:null;e=null}else t!==e&&(e=null);return Na=e,null}function tm(e){switch(e){case"cancel":case"click":case"close":case"contextmenu":case"copy":case"cut":case"auxclick":case"dblclick":case"dragend":case"dragstart":case"drop":case"focusin":case"focusout":case"input":case"invalid":case"keydown":case"keypress":case"keyup":case"mousedown":case"mouseup":case"paste":case"pause":case"play":case"pointercancel":case"pointerdown":case"pointerup":case"ratechange":case"reset":case"resize":case"seeked":case"submit":case"touchcancel":case"touchend":case"touchstart":case"volumechange":case"change":case"selectionchange":case"textInput":case"compositionstart":case"compositionend":case"compositionupdate":case"beforeblur":case"afterblur":case"beforeinput":case"blur":case"fullscreenchange":case"focus":case"hashchange":case"popstate":case"select":case"selectstart":return 1;case"drag":case"dragenter":case"dragexit":case"dragleave":case"dragover":case"mousemove":case"mouseout":case"mouseover":case"pointermove":case"pointerout":case"pointerover":case"scroll":case"toggle":case"touchmove":case"wheel":case"mouseenter":case"mouseleave":case"pointerenter":case"pointerleave":return 4;case"message":switch(ry()){case Hu:return 1;case Qp:return 4;case Ma:case iy:return 16;case Vp:return 536870912;default:return 16}default:return 16}}var sr=null,Vu=null,ha=null;function nm(){if(ha)return ha;var e,t=Vu,n=t.length,o,r="value"in sr?sr.value:sr.textContent,i=r.length;for(e=0;e<n&&t[e]===r[e];e++);var l=n-e;for(o=1;o<=l&&t[n-o]===r[i-o];o++);return ha=r.slice(e,1<o?1-o:void 0)}function pa(e){var t=e.keyCode;return"charCode"in e?(e=e.charCode,e===0&&t===13&&(e=13)):e=t,e===10&&(e=13),32<=e||e===13?e:0}function ea(){return!0}function Rh(){return!1}function Dn(e){function t(n,o,r,i,l){this._reactName=n,this._targetInst=r,this.type=o,this.nativeEvent=i,this.target=l,this.currentTarget=null;for(var s in e)e.hasOwnProperty(s)&&(n=e[s],this[s]=n?n(i):i[s]);return this.isDefaultPrevented=(i.defaultPrevented!=null?i.defaultPrevented:i.returnValue===!1)?ea:Rh,this.isPropagationStopped=Rh,this}return Rt(t.prototype,{preventDefault:function(){this.defaultPrevented=!0;var n=this.nativeEvent;n&&(n.preventDefault?n.preventDefault():typeof n.returnValue!="unknown"&&(n.returnValue=!1),this.isDefaultPrevented=ea)},stopPropagation:function(){var n=this.nativeEvent;n&&(n.stopPropagation?n.stopPropagation():typeof n.cancelBubble!="unknown"&&(n.cancelBubble=!0),this.isPropagationStopped=ea)},persist:function(){},isPersistent:ea}),t}var Yi={eventPhase:0,bubbles:0,cancelable:0,timeStamp:function(e){return e.timeStamp||Date.now()},defaultPrevented:0,isTrusted:0},Xu=Dn(Yi),is=Rt({},Yi,{view:0,detail:0}),yy=Dn(is),Ed,Ld,kl,Xa=Rt({},is,{screenX:0,screenY:0,clientX:0,clientY:0,pageX:0,pageY:0,ctrlKey:0,shiftKey:0,altKey:0,metaKey:0,getModifierState:Gu,button:0,buttons:0,relatedTarget:function(e){return e.relatedTarget===void 0?e.fromElement===e.srcElement?e.toElement:e.fromElement:e.relatedTarget},movementX:function(e){return"movementX"in e?e.movementX:(e!==kl&&(kl&&e.type==="mousemove"?(Ed=e.screenX-kl.screenX,Ld=e.screenY-kl.screenY):Ld=Ed=0,kl=e),Ed)},movementY:function(e){return"movementY"in e?e.movementY:Ld}}),$h=Dn(Xa),xy=Rt({},Xa,{dataTransfer:0}),vy=Dn(xy),wy=Rt({},is,{relatedTarget:0}),Nd=Dn(wy),by=Rt({},Yi,{animationName:0,elapsedTime:0,pseudoElement:0}),ky=Dn(by),Cy=Rt({},Yi,{clipboardData:function(e){return"clipboardData"in e?e.clipboardData:window.clipboardData}}),Sy=Dn(Cy),My=Rt({},Yi,{data:0}),Th=Dn(My),Ey={Esc:"Escape",Spacebar:" ",Left:"ArrowLeft",Up:"ArrowUp",Right:"ArrowRight",Down:"ArrowDown",Del:"Delete",Win:"OS",Menu:"ContextMenu",Apps:"ContextMenu",Scroll:"ScrollLock",MozPrintableKey:"Unidentified"},Ly={8:"Backspace",9:"Tab",12:"Clear",13:"Enter",16:"Shift",17:"Control",18:"Alt",19:"Pause",20:"CapsLock",27:"Escape",32:" ",33:"PageUp",34:"PageDown",35:"End",36:"Home",37:"ArrowLeft",38:"ArrowUp",39:"ArrowRight",40:"ArrowDown",45:"Insert",46:"Delete",112:"F1",113:"F2",114:"F3",115:"F4",116:"F5",117:"F6",118:"F7",119:"F8",120:"F9",121:"F10",122:"F11",123:"F12",144:"NumLock",145:"ScrollLock",224:"Meta"},Ny={Alt:"altKey",Control:"ctrlKey",Meta:"metaKey",Shift:"shiftKey"};function Iy(e){var t=this.nativeEvent;return t.getModifierState?t.getModifierState(e):(e=Ny[e])?!!t[e]:!1}function Gu(){return Iy}var Ry=Rt({},is,{key:function(e){if(e.key){var t=Ey[e.key]||e.key;if(t!=="Unidentified")return t}return e.type==="keypress"?(e=pa(e),e===13?"Enter":String.fromCharCode(e)):e.type==="keydown"||e.type==="keyup"?Ly[e.keyCode]||"Unidentified":""},code:0,location:0,ctrlKey:0,shiftKey:0,altKey:0,metaKey:0,repeat:0,locale:0,getModifierState:Gu,charCode:function(e){return e.type==="keypress"?pa(e):0},keyCode:function(e){return e.type==="keydown"||e.type==="keyup"?e.keyCode:0},which:function(e){return e.type==="keypress"?pa(e):e.type==="keydown"||e.type==="keyup"?e.keyCode:0}}),$y=Dn(Ry),Ty=Rt({},Xa,{pointerId:0,width:0,height:0,pressure:0,tangentialPressure:0,tiltX:0,tiltY:0,twist:0,pointerType:0,isPrimary:0}),Ph=Dn(Ty),Py=Rt({},is,{touches:0,targetTouches:0,changedTouches:0,altKey:0,metaKey:0,ctrlKey:0,shiftKey:0,getModifierState:Gu}),Dy=Dn(Py),By=Rt({},Yi,{propertyName:0,elapsedTime:0,pseudoElement:0}),zy=Dn(By),Oy=Rt({},Xa,{deltaX:function(e){return"deltaX"in e?e.deltaX:"wheelDeltaX"in e?-e.wheelDeltaX:0},deltaY:function(e){return"deltaY"in e?e.deltaY:"wheelDeltaY"in e?-e.wheelDeltaY:"wheelDelta"in e?-e.wheelDelta:0},deltaZ:0,deltaMode:0}),Ay=Dn(Oy),Fy=[9,13,27,32],qu=Yo&&"CompositionEvent"in window,Pl=null;Yo&&"documentMode"in document&&(Pl=document.documentMode);var Wy=Yo&&"TextEvent"in window&&!Pl,om=Yo&&(!qu||Pl&&8<Pl&&11>=Pl),Dh=" ",Bh=!1;function rm(e,t){switch(e){case"keyup":return Fy.indexOf(t.keyCode)!==-1;case"keydown":return t.keyCode!==229;case"keypress":case"mousedown":case"focusout":return!0;default:return!1}}function im(e){return e=e.detail,typeof e=="object"&&"data"in e?e.data:null}var bi=!1;function jy(e,t){switch(e){case"compositionend":return im(t);case"keypress":return t.which!==32?null:(Bh=!0,Dh);case"textInput":return e=t.data,e===Dh&&Bh?null:e;default:return null}}function Hy(e,t){if(bi)return e==="compositionend"||!qu&&rm(e,t)?(e=nm(),ha=Vu=sr=null,bi=!1,e):null;switch(e){case"paste":return null;case"keypress":if(!(t.ctrlKey||t.altKey||t.metaKey)||t.ctrlKey&&t.altKey){if(t.char&&1<t.char.length)return t.char;if(t.which)return String.fromCharCode(t.which)}return null;case"compositionend":return om&&t.locale!=="ko"?null:t.data;default:return null}}var Uy={color:!0,date:!0,datetime:!0,"datetime-local":!0,email:!0,month:!0,number:!0,password:!0,range:!0,search:!0,tel:!0,text:!0,time:!0,url:!0,week:!0};function zh(e){var t=e&&e.nodeName&&e.nodeName.toLowerCase();return t==="input"?!!Uy[e.type]:t==="textarea"}function lm(e,t,n,o){zp(o),t=Ia(t,"onChange"),0<t.length&&(n=new Xu("onChange","change",null,n,o),e.push({event:n,listeners:t}))}var Dl=null,Vl=null;function Yy(e){gm(e,0)}function Ga(e){var t=Si(e);if(Ip(t))return e}function Qy(e,t){if(e==="change")return t}var sm=!1;Yo&&(Yo?(na="oninput"in document,na||(Id=document.createElement("div"),Id.setAttribute("oninput","return;"),na=typeof Id.oninput=="function"),ta=na):ta=!1,sm=ta&&(!document.documentMode||9<document.documentMode));var ta,na,Id;function Oh(){Dl&&(Dl.detachEvent("onpropertychange",am),Vl=Dl=null)}function am(e){if(e.propertyName==="value"&&Ga(Vl)){var t=[];lm(t,Vl,e,ju(e)),Wp(Yy,t)}}function Vy(e,t,n){e==="focusin"?(Oh(),Dl=t,Vl=n,Dl.attachEvent("onpropertychange",am)):e==="focusout"&&Oh()}function Xy(e){if(e==="selectionchange"||e==="keyup"||e==="keydown")return Ga(Vl)}function Gy(e,t){if(e==="click")return Ga(t)}function qy(e,t){if(e==="input"||e==="change")return Ga(t)}function Ky(e,t){return e===t&&(e!==0||1/e===1/t)||e!==e&&t!==t}var ho=typeof Object.is=="function"?Object.is:Ky;function Xl(e,t){if(ho(e,t))return!0;if(typeof e!="object"||e===null||typeof t!="object"||t===null)return!1;var n=Object.keys(e),o=Object.keys(t);if(n.length!==o.length)return!1;for(o=0;o<n.length;o++){var r=n[o];if(!Ud.call(t,r)||!ho(e[r],t[r]))return!1}return!0}function Ah(e){for(;e&&e.firstChild;)e=e.firstChild;return e}function Fh(e,t){var n=Ah(e);e=0;for(var o;n;){if(n.nodeType===3){if(o=e+n.textContent.length,e<=t&&o>=t)return{node:n,offset:t-e};e=o}e:{for(;n;){if(n.nextSibling){n=n.nextSibling;break e}n=n.parentNode}n=void 0}n=Ah(n)}}function cm(e,t){return e&&t?e===t?!0:e&&e.nodeType===3?!1:t&&t.nodeType===3?cm(e,t.parentNode):"contains"in e?e.contains(t):e.compareDocumentPosition?!!(e.compareDocumentPosition(t)&16):!1:!1}function dm(){for(var e=window,t=ka();t instanceof e.HTMLIFrameElement;){try{var n=typeof t.contentWindow.location.href=="string"}catch{n=!1}if(n)e=t.contentWindow;else break;t=ka(e.document)}return t}function Ku(e){var t=e&&e.nodeName&&e.nodeName.toLowerCase();return t&&(t==="input"&&(e.type==="text"||e.type==="search"||e.type==="tel"||e.type==="url"||e.type==="password")||t==="textarea"||e.contentEditable==="true")}function Jy(e){var t=dm(),n=e.focusedElem,o=e.selectionRange;if(t!==n&&n&&n.ownerDocument&&cm(n.ownerDocument.documentElement,n)){if(o!==null&&Ku(n)){if(t=o.start,e=o.end,e===void 0&&(e=t),"selectionStart"in n)n.selectionStart=t,n.selectionEnd=Math.min(e,n.value.length);else if(e=(t=n.ownerDocument||document)&&t.defaultView||window,e.getSelection){e=e.getSelection();var r=n.textContent.length,i=Math.min(o.start,r);o=o.end===void 0?i:Math.min(o.end,r),!e.extend&&i>o&&(r=o,o=i,i=r),r=Fh(n,i);var l=Fh(n,o);r&&l&&(e.rangeCount!==1||e.anchorNode!==r.node||e.anchorOffset!==r.offset||e.focusNode!==l.node||e.focusOffset!==l.offset)&&(t=t.createRange(),t.setStart(r.node,r.offset),e.removeAllRanges(),i>o?(e.addRange(t),e.extend(l.node,l.offset)):(t.setEnd(l.node,l.offset),e.addRange(t)))}}for(t=[],e=n;e=e.parentNode;)e.nodeType===1&&t.push({element:e,left:e.scrollLeft,top:e.scrollTop});for(typeof n.focus=="function"&&n.focus(),n=0;n<t.length;n++)e=t[n],e.element.scrollLeft=e.left,e.element.scrollTop=e.top}}var Zy=Yo&&"documentMode"in document&&11>=document.documentMode,ki=null,cu=null,Bl=null,du=!1;function Wh(e,t,n){var o=n.window===n?n.document:n.nodeType===9?n:n.ownerDocument;du||ki==null||ki!==ka(o)||(o=ki,"selectionStart"in o&&Ku(o)?o={start:o.selectionStart,end:o.selectionEnd}:(o=(o.ownerDocument&&o.ownerDocument.defaultView||window).getSelection(),o={anchorNode:o.anchorNode,anchorOffset:o.anchorOffset,focusNode:o.focusNode,focusOffset:o.focusOffset}),Bl&&Xl(Bl,o)||(Bl=o,o=Ia(cu,"onSelect"),0<o.length&&(t=new Xu("onSelect","select",null,t,n),e.push({event:t,listeners:o}),t.target=ki)))}function oa(e,t){var n={};return n[e.toLowerCase()]=t.toLowerCase(),n["Webkit"+e]="webkit"+t,n["Moz"+e]="moz"+t,n}var Ci={animationend:oa("Animation","AnimationEnd"),animationiteration:oa("Animation","AnimationIteration"),animationstart:oa("Animation","AnimationStart"),transitionend:oa("Transition","TransitionEnd")},Rd={},um={};Yo&&(um=document.createElement("div").style,"AnimationEvent"in window||(delete Ci.animationend.animation,delete Ci.animationiteration.animation,delete Ci.animationstart.animation),"TransitionEvent"in window||delete Ci.transitionend.transition);function qa(e){if(Rd[e])return Rd[e];if(!Ci[e])return e;var t=Ci[e],n;for(n in t)if(t.hasOwnProperty(n)&&n in um)return Rd[e]=t[n];return e}var _m=qa("animationend"),fm=qa("animationiteration"),hm=qa("animationstart"),pm=qa("transitionend"),mm=new Map,jh="abort auxClick cancel canPlay canPlayThrough click close contextMenu copy cut drag dragEnd dragEnter dragExit dragLeave dragOver dragStart drop durationChange emptied encrypted ended error gotPointerCapture input invalid keyDown keyPress keyUp load loadedData loadedMetadata loadStart lostPointerCapture mouseDown mouseMove mouseOut mouseOver mouseUp paste pause play playing pointerCancel pointerDown pointerMove pointerOut pointerOver pointerUp progress rateChange reset resize seeked seeking stalled submit suspend timeUpdate touchCancel touchEnd touchStart volumeChange scroll toggle touchMove waiting wheel".split(" ");function xr(e,t){mm.set(e,t),ti(t,[e])}for(ra=0;ra<jh.length;ra++)ia=jh[ra],Hh=ia.toLowerCase(),Uh=ia[0].toUpperCase()+ia.slice(1),xr(Hh,"on"+Uh);var ia,Hh,Uh,ra;xr(_m,"onAnimationEnd");xr(fm,"onAnimationIteration");xr(hm,"onAnimationStart");xr("dblclick","onDoubleClick");xr("focusin","onFocus");xr("focusout","onBlur");xr(pm,"onTransitionEnd");Oi("onMouseEnter",["mouseout","mouseover"]);Oi("onMouseLeave",["mouseout","mouseover"]);Oi("onPointerEnter",["pointerout","pointerover"]);Oi("onPointerLeave",["pointerout","pointerover"]);ti("onChange","change click focusin focusout input keydown keyup selectionchange".split(" "));ti("onSelect","focusout contextmenu dragend focusin keydown keyup mousedown mouseup selectionchange".split(" "));ti("onBeforeInput",["compositionend","keypress","textInput","paste"]);ti("onCompositionEnd","compositionend focusout keydown keypress keyup mousedown".split(" "));ti("onCompositionStart","compositionstart focusout keydown keypress keyup mousedown".split(" "));ti("onCompositionUpdate","compositionupdate focusout keydown keypress keyup mousedown".split(" "));var Rl="abort canplay canplaythrough durationchange emptied encrypted ended error loadeddata loadedmetadata loadstart pause play playing progress ratechange resize seeked seeking stalled suspend timeupdate volumechange waiting".split(" "),e5=new Set("cancel close invalid load scroll toggle".split(" ").concat(Rl));function Yh(e,t,n){var o=e.type||"unknown-event";e.currentTarget=n,ey(o,t,void 0,e),e.currentTarget=null}function gm(e,t){t=(t&4)!==0;for(var n=0;n<e.length;n++){var o=e[n],r=o.event;o=o.listeners;e:{var i=void 0;if(t)for(var l=o.length-1;0<=l;l--){var s=o[l],a=s.instance,c=s.currentTarget;if(s=s.listener,a!==i&&r.isPropagationStopped())break e;Yh(r,s,c),i=a}else for(l=0;l<o.length;l++){if(s=o[l],a=s.instance,c=s.currentTarget,s=s.listener,a!==i&&r.isPropagationStopped())break e;Yh(r,s,c),i=a}}}if(Sa)throw e=iu,Sa=!1,iu=null,e}function St(e,t){var n=t[pu];n===void 0&&(n=t[pu]=new Set);var o=e+"__bubble";n.has(o)||(ym(t,e,2,!1),n.add(o))}function $d(e,t,n){var o=0;t&&(o|=4),ym(n,e,o,t)}var la="_reactListening"+Math.random().toString(36).slice(2);function Gl(e){if(!e[la]){e[la]=!0,Sp.forEach(function(n){n!=="selectionchange"&&(e5.has(n)||$d(n,!1,e),$d(n,!0,e))});var t=e.nodeType===9?e:e.ownerDocument;t===null||t[la]||(t[la]=!0,$d("selectionchange",!1,t))}}function ym(e,t,n,o){switch(tm(t)){case 1:var r=my;break;case 4:r=gy;break;default:r=Qu}n=r.bind(null,t,n,e),r=void 0,!ru||t!=="touchstart"&&t!=="touchmove"&&t!=="wheel"||(r=!0),o?r!==void 0?e.addEventListener(t,n,{capture:!0,passive:r}):e.addEventListener(t,n,!0):r!==void 0?e.addEventListener(t,n,{passive:r}):e.addEventListener(t,n,!1)}function Td(e,t,n,o,r){var i=o;if((t&1)===0&&(t&2)===0&&o!==null)e:for(;;){if(o===null)return;var l=o.tag;if(l===3||l===4){var s=o.stateNode.containerInfo;if(s===r||s.nodeType===8&&s.parentNode===r)break;if(l===4)for(l=o.return;l!==null;){var a=l.tag;if((a===3||a===4)&&(a=l.stateNode.containerInfo,a===r||a.nodeType===8&&a.parentNode===r))return;l=l.return}for(;s!==null;){if(l=Yr(s),l===null)return;if(a=l.tag,a===5||a===6){o=i=l;continue e}s=s.parentNode}}o=o.return}Wp(function(){var c=i,f=ju(n),u=[];e:{var x=mm.get(e);if(x!==void 0){var S=Xu,b=e;switch(e){case"keypress":if(pa(n)===0)break e;case"keydown":case"keyup":S=$y;break;case"focusin":b="focus",S=Nd;break;case"focusout":b="blur",S=Nd;break;case"beforeblur":case"afterblur":S=Nd;break;case"click":if(n.button===2)break e;case"auxclick":case"dblclick":case"mousedown":case"mousemove":case"mouseup":case"mouseout":case"mouseover":case"contextmenu":S=$h;break;case"drag":case"dragend":case"dragenter":case"dragexit":case"dragleave":case"dragover":case"dragstart":case"drop":S=vy;break;case"touchcancel":case"touchend":case"touchmove":case"touchstart":S=Dy;break;case _m:case fm:case hm:S=ky;break;case pm:S=zy;break;case"scroll":S=yy;break;case"wheel":S=Ay;break;case"copy":case"cut":case"paste":S=Sy;break;case"gotpointercapture":case"lostpointercapture":case"pointercancel":case"pointerdown":case"pointermove":case"pointerout":case"pointerover":case"pointerup":S=Ph}var N=(t&4)!==0,E=!N&&e==="scroll",g=N?x!==null?x+"Capture":null:x;N=[];for(var v=c,h;v!==null;){h=v;var C=h.stateNode;if(h.tag===5&&C!==null&&(h=C,g!==null&&(C=Hl(v,g),C!=null&&N.push(ql(v,C,h)))),E)break;v=v.return}0<N.length&&(x=new S(x,b,null,n,f),u.push({event:x,listeners:N}))}}if((t&7)===0){e:{if(x=e==="mouseover"||e==="pointerover",S=e==="mouseout"||e==="pointerout",x&&n!==nu&&(b=n.relatedTarget||n.fromElement)&&(Yr(b)||b[Qo]))break e;if((S||x)&&(x=f.window===f?f:(x=f.ownerDocument)?x.defaultView||x.parentWindow:window,S?(b=n.relatedTarget||n.toElement,S=c,b=b?Yr(b):null,b!==null&&(E=ni(b),b!==E||b.tag!==5&&b.tag!==6)&&(b=null)):(S=null,b=c),S!==b)){if(N=$h,C="onMouseLeave",g="onMouseEnter",v="mouse",(e==="pointerout"||e==="pointerover")&&(N=Ph,C="onPointerLeave",g="onPointerEnter",v="pointer"),E=S==null?x:Si(S),h=b==null?x:Si(b),x=new N(C,v+"leave",S,n,f),x.target=E,x.relatedTarget=h,C=null,Yr(f)===c&&(N=new N(g,v+"enter",b,n,f),N.target=h,N.relatedTarget=E,C=N),E=C,S&&b)t:{for(N=S,g=b,v=0,h=N;h;h=xi(h))v++;for(h=0,C=g;C;C=xi(C))h++;for(;0<v-h;)N=xi(N),v--;for(;0<h-v;)g=xi(g),h--;for(;v--;){if(N===g||g!==null&&N===g.alternate)break t;N=xi(N),g=xi(g)}N=null}else N=null;S!==null&&Qh(u,x,S,N,!1),b!==null&&E!==null&&Qh(u,E,b,N,!0)}}e:{if(x=c?Si(c):window,S=x.nodeName&&x.nodeName.toLowerCase(),S==="select"||S==="input"&&x.type==="file")var U=Qy;else if(zh(x))if(sm)U=qy;else{U=Xy;var V=Vy}else(S=x.nodeName)&&S.toLowerCase()==="input"&&(x.type==="checkbox"||x.type==="radio")&&(U=Gy);if(U&&(U=U(e,c))){lm(u,U,n,f);break e}V&&V(e,x,c),e==="focusout"&&(V=x._wrapperState)&&V.controlled&&x.type==="number"&&Kd(x,"number",x.value)}switch(V=c?Si(c):window,e){case"focusin":(zh(V)||V.contentEditable==="true")&&(ki=V,cu=c,Bl=null);break;case"focusout":Bl=cu=ki=null;break;case"mousedown":du=!0;break;case"contextmenu":case"mouseup":case"dragend":du=!1,Wh(u,n,f);break;case"selectionchange":if(Zy)break;case"keydown":case"keyup":Wh(u,n,f)}var B;if(qu)e:{switch(e){case"compositionstart":var q="onCompositionStart";break e;case"compositionend":q="onCompositionEnd";break e;case"compositionupdate":q="onCompositionUpdate";break e}q=void 0}else bi?rm(e,n)&&(q="onCompositionEnd"):e==="keydown"&&n.keyCode===229&&(q="onCompositionStart");q&&(om&&n.locale!=="ko"&&(bi||q!=="onCompositionStart"?q==="onCompositionEnd"&&bi&&(B=nm()):(sr=f,Vu="value"in sr?sr.value:sr.textContent,bi=!0)),V=Ia(c,q),0<V.length&&(q=new Th(q,e,null,n,f),u.push({event:q,listeners:V}),B?q.data=B:(B=im(n),B!==null&&(q.data=B)))),(B=Wy?jy(e,n):Hy(e,n))&&(c=Ia(c,"onBeforeInput"),0<c.length&&(f=new Th("onBeforeInput","beforeinput",null,n,f),u.push({event:f,listeners:c}),f.data=B))}gm(u,t)})}function ql(e,t,n){return{instance:e,listener:t,currentTarget:n}}function Ia(e,t){for(var n=t+"Capture",o=[];e!==null;){var r=e,i=r.stateNode;r.tag===5&&i!==null&&(r=i,i=Hl(e,n),i!=null&&o.unshift(ql(e,i,r)),i=Hl(e,t),i!=null&&o.push(ql(e,i,r))),e=e.return}return o}function xi(e){if(e===null)return null;do e=e.return;while(e&&e.tag!==5);return e||null}function Qh(e,t,n,o,r){for(var i=t._reactName,l=[];n!==null&&n!==o;){var s=n,a=s.alternate,c=s.stateNode;if(a!==null&&a===o)break;s.tag===5&&c!==null&&(s=c,r?(a=Hl(n,i),a!=null&&l.unshift(ql(n,a,s))):r||(a=Hl(n,i),a!=null&&l.push(ql(n,a,s)))),n=n.return}l.length!==0&&e.push({event:t,listeners:l})}var t5=/\r\n?/g,n5=/\u0000|\uFFFD/g;function Vh(e){return(typeof e=="string"?e:""+e).replace(t5,`
`).replace(n5,"")}function sa(e,t,n){if(t=Vh(t),Vh(e)!==t&&n)throw Error(Q(425))}function Ra(){}var uu=null,_u=null;function fu(e,t){return e==="textarea"||e==="noscript"||typeof t.children=="string"||typeof t.children=="number"||typeof t.dangerouslySetInnerHTML=="object"&&t.dangerouslySetInnerHTML!==null&&t.dangerouslySetInnerHTML.__html!=null}var hu=typeof setTimeout=="function"?setTimeout:void 0,o5=typeof clearTimeout=="function"?clearTimeout:void 0,Xh=typeof Promise=="function"?Promise:void 0,r5=typeof queueMicrotask=="function"?queueMicrotask:typeof Xh<"u"?function(e){return Xh.resolve(null).then(e).catch(i5)}:hu;function i5(e){setTimeout(function(){throw e})}function Pd(e,t){var n=t,o=0;do{var r=n.nextSibling;if(e.removeChild(n),r&&r.nodeType===8)if(n=r.data,n==="/$"){if(o===0){e.removeChild(r),Ql(t);return}o--}else n!=="$"&&n!=="$?"&&n!=="$!"||o++;n=r}while(n);Ql(t)}function _r(e){for(;e!=null;e=e.nextSibling){var t=e.nodeType;if(t===1||t===3)break;if(t===8){if(t=e.data,t==="$"||t==="$!"||t==="$?")break;if(t==="/$")return null}}return e}function Gh(e){e=e.previousSibling;for(var t=0;e;){if(e.nodeType===8){var n=e.data;if(n==="$"||n==="$!"||n==="$?"){if(t===0)return e;t--}else n==="/$"&&t++}e=e.previousSibling}return null}var Qi=Math.random().toString(36).slice(2),Mo="__reactFiber$"+Qi,Kl="__reactProps$"+Qi,Qo="__reactContainer$"+Qi,pu="__reactEvents$"+Qi,l5="__reactListeners$"+Qi,s5="__reactHandles$"+Qi;function Yr(e){var t=e[Mo];if(t)return t;for(var n=e.parentNode;n;){if(t=n[Qo]||n[Mo]){if(n=t.alternate,t.child!==null||n!==null&&n.child!==null)for(e=Gh(e);e!==null;){if(n=e[Mo])return n;e=Gh(e)}return t}e=n,n=e.parentNode}return null}function ls(e){return e=e[Mo]||e[Qo],!e||e.tag!==5&&e.tag!==6&&e.tag!==13&&e.tag!==3?null:e}function Si(e){if(e.tag===5||e.tag===6)return e.stateNode;throw Error(Q(33))}function Ka(e){return e[Kl]||null}var mu=[],Mi=-1;function vr(e){return{current:e}}function Mt(e){0>Mi||(e.current=mu[Mi],mu[Mi]=null,Mi--)}function wt(e,t){Mi++,mu[Mi]=e.current,e.current=t}var yr={},fn=vr(yr),Mn=vr(!1),qr=yr;function Ai(e,t){var n=e.type.contextTypes;if(!n)return yr;var o=e.stateNode;if(o&&o.__reactInternalMemoizedUnmaskedChildContext===t)return o.__reactInternalMemoizedMaskedChildContext;var r={},i;for(i in n)r[i]=t[i];return o&&(e=e.stateNode,e.__reactInternalMemoizedUnmaskedChildContext=t,e.__reactInternalMemoizedMaskedChildContext=r),r}function En(e){return e=e.childContextTypes,e!=null}function $a(){Mt(Mn),Mt(fn)}function qh(e,t,n){if(fn.current!==yr)throw Error(Q(168));wt(fn,t),wt(Mn,n)}function xm(e,t,n){var o=e.stateNode;if(t=t.childContextTypes,typeof o.getChildContext!="function")return n;o=o.getChildContext();for(var r in o)if(!(r in t))throw Error(Q(108,V1(e)||"Unknown",r));return Rt({},n,o)}function Ta(e){return e=(e=e.stateNode)&&e.__reactInternalMemoizedMergedChildContext||yr,qr=fn.current,wt(fn,e),wt(Mn,Mn.current),!0}function Kh(e,t,n){var o=e.stateNode;if(!o)throw Error(Q(169));n?(e=xm(e,t,qr),o.__reactInternalMemoizedMergedChildContext=e,Mt(Mn),Mt(fn),wt(fn,e)):Mt(Mn),wt(Mn,n)}var Wo=null,Ja=!1,Dd=!1;function vm(e){Wo===null?Wo=[e]:Wo.push(e)}function a5(e){Ja=!0,vm(e)}function wr(){if(!Dd&&Wo!==null){Dd=!0;var e=0,t=dt;try{var n=Wo;for(dt=1;e<n.length;e++){var o=n[e];do o=o(!0);while(o!==null)}Wo=null,Ja=!1}catch(r){throw Wo!==null&&(Wo=Wo.slice(e+1)),Yp(Hu,wr),r}finally{dt=t,Dd=!1}}return null}var Ei=[],Li=0,Pa=null,Da=0,Qn=[],Vn=0,Kr=null,jo=1,Ho="";function Hr(e,t){Ei[Li++]=Da,Ei[Li++]=Pa,Pa=e,Da=t}function wm(e,t,n){Qn[Vn++]=jo,Qn[Vn++]=Ho,Qn[Vn++]=Kr,Kr=e;var o=jo;e=Ho;var r=32-_o(o)-1;o&=~(1<<r),n+=1;var i=32-_o(t)+r;if(30<i){var l=r-r%5;i=(o&(1<<l)-1).toString(32),o>>=l,r-=l,jo=1<<32-_o(t)+r|n<<r|o,Ho=i+e}else jo=1<<i|n<<r|o,Ho=e}function Ju(e){e.return!==null&&(Hr(e,1),wm(e,1,0))}function Zu(e){for(;e===Pa;)Pa=Ei[--Li],Ei[Li]=null,Da=Ei[--Li],Ei[Li]=null;for(;e===Kr;)Kr=Qn[--Vn],Qn[Vn]=null,Ho=Qn[--Vn],Qn[Vn]=null,jo=Qn[--Vn],Qn[Vn]=null}var Tn=null,$n=null,Lt=!1,uo=null;function bm(e,t){var n=Xn(5,null,null,0);n.elementType="DELETED",n.stateNode=t,n.return=e,t=e.deletions,t===null?(e.deletions=[n],e.flags|=16):t.push(n)}function Jh(e,t){switch(e.tag){case 5:var n=e.type;return t=t.nodeType!==1||n.toLowerCase()!==t.nodeName.toLowerCase()?null:t,t!==null?(e.stateNode=t,Tn=e,$n=_r(t.firstChild),!0):!1;case 6:return t=e.pendingProps===""||t.nodeType!==3?null:t,t!==null?(e.stateNode=t,Tn=e,$n=null,!0):!1;case 13:return t=t.nodeType!==8?null:t,t!==null?(n=Kr!==null?{id:jo,overflow:Ho}:null,e.memoizedState={dehydrated:t,treeContext:n,retryLane:1073741824},n=Xn(18,null,null,0),n.stateNode=t,n.return=e,e.child=n,Tn=e,$n=null,!0):!1;default:return!1}}function gu(e){return(e.mode&1)!==0&&(e.flags&128)===0}function yu(e){if(Lt){var t=$n;if(t){var n=t;if(!Jh(e,t)){if(gu(e))throw Error(Q(418));t=_r(n.nextSibling);var o=Tn;t&&Jh(e,t)?bm(o,n):(e.flags=e.flags&-4097|2,Lt=!1,Tn=e)}}else{if(gu(e))throw Error(Q(418));e.flags=e.flags&-4097|2,Lt=!1,Tn=e}}}function Zh(e){for(e=e.return;e!==null&&e.tag!==5&&e.tag!==3&&e.tag!==13;)e=e.return;Tn=e}function aa(e){if(e!==Tn)return!1;if(!Lt)return Zh(e),Lt=!0,!1;var t;if((t=e.tag!==3)&&!(t=e.tag!==5)&&(t=e.type,t=t!=="head"&&t!=="body"&&!fu(e.type,e.memoizedProps)),t&&(t=$n)){if(gu(e))throw km(),Error(Q(418));for(;t;)bm(e,t),t=_r(t.nextSibling)}if(Zh(e),e.tag===13){if(e=e.memoizedState,e=e!==null?e.dehydrated:null,!e)throw Error(Q(317));e:{for(e=e.nextSibling,t=0;e;){if(e.nodeType===8){var n=e.data;if(n==="/$"){if(t===0){$n=_r(e.nextSibling);break e}t--}else n!=="$"&&n!=="$!"&&n!=="$?"||t++}e=e.nextSibling}$n=null}}else $n=Tn?_r(e.stateNode.nextSibling):null;return!0}function km(){for(var e=$n;e;)e=_r(e.nextSibling)}function Fi(){$n=Tn=null,Lt=!1}function e_(e){uo===null?uo=[e]:uo.push(e)}var c5=Go.ReactCurrentBatchConfig;function Cl(e,t,n){if(e=n.ref,e!==null&&typeof e!="function"&&typeof e!="object"){if(n._owner){if(n=n._owner,n){if(n.tag!==1)throw Error(Q(309));var o=n.stateNode}if(!o)throw Error(Q(147,e));var r=o,i=""+e;return t!==null&&t.ref!==null&&typeof t.ref=="function"&&t.ref._stringRef===i?t.ref:(t=function(l){var s=r.refs;l===null?delete s[i]:s[i]=l},t._stringRef=i,t)}if(typeof e!="string")throw Error(Q(284));if(!n._owner)throw Error(Q(290,e))}return e}function ca(e,t){throw e=Object.prototype.toString.call(t),Error(Q(31,e==="[object Object]"?"object with keys {"+Object.keys(t).join(", ")+"}":e))}function ep(e){var t=e._init;return t(e._payload)}function Cm(e){function t(g,v){if(e){var h=g.deletions;h===null?(g.deletions=[v],g.flags|=16):h.push(v)}}function n(g,v){if(!e)return null;for(;v!==null;)t(g,v),v=v.sibling;return null}function o(g,v){for(g=new Map;v!==null;)v.key!==null?g.set(v.key,v):g.set(v.index,v),v=v.sibling;return g}function r(g,v){return g=mr(g,v),g.index=0,g.sibling=null,g}function i(g,v,h){return g.index=h,e?(h=g.alternate,h!==null?(h=h.index,h<v?(g.flags|=2,v):h):(g.flags|=2,v)):(g.flags|=1048576,v)}function l(g){return e&&g.alternate===null&&(g.flags|=2),g}function s(g,v,h,C){return v===null||v.tag!==6?(v=jd(h,g.mode,C),v.return=g,v):(v=r(v,h),v.return=g,v)}function a(g,v,h,C){var U=h.type;return U===wi?f(g,v,h.props.children,C,h.key):v!==null&&(v.elementType===U||typeof U=="object"&&U!==null&&U.$$typeof===or&&ep(U)===v.type)?(C=r(v,h.props),C.ref=Cl(g,v,h),C.return=g,C):(C=ba(h.type,h.key,h.props,null,g.mode,C),C.ref=Cl(g,v,h),C.return=g,C)}function c(g,v,h,C){return v===null||v.tag!==4||v.stateNode.containerInfo!==h.containerInfo||v.stateNode.implementation!==h.implementation?(v=Hd(h,g.mode,C),v.return=g,v):(v=r(v,h.children||[]),v.return=g,v)}function f(g,v,h,C,U){return v===null||v.tag!==7?(v=Gr(h,g.mode,C,U),v.return=g,v):(v=r(v,h),v.return=g,v)}function u(g,v,h){if(typeof v=="string"&&v!==""||typeof v=="number")return v=jd(""+v,g.mode,h),v.return=g,v;if(typeof v=="object"&&v!==null){switch(v.$$typeof){case Xs:return h=ba(v.type,v.key,v.props,null,g.mode,h),h.ref=Cl(g,null,v),h.return=g,h;case vi:return v=Hd(v,g.mode,h),v.return=g,v;case or:var C=v._init;return u(g,C(v._payload),h)}if(Nl(v)||vl(v))return v=Gr(v,g.mode,h,null),v.return=g,v;ca(g,v)}return null}function x(g,v,h,C){var U=v!==null?v.key:null;if(typeof h=="string"&&h!==""||typeof h=="number")return U!==null?null:s(g,v,""+h,C);if(typeof h=="object"&&h!==null){switch(h.$$typeof){case Xs:return h.key===U?a(g,v,h,C):null;case vi:return h.key===U?c(g,v,h,C):null;case or:return U=h._init,x(g,v,U(h._payload),C)}if(Nl(h)||vl(h))return U!==null?null:f(g,v,h,C,null);ca(g,h)}return null}function S(g,v,h,C,U){if(typeof C=="string"&&C!==""||typeof C=="number")return g=g.get(h)||null,s(v,g,""+C,U);if(typeof C=="object"&&C!==null){switch(C.$$typeof){case Xs:return g=g.get(C.key===null?h:C.key)||null,a(v,g,C,U);case vi:return g=g.get(C.key===null?h:C.key)||null,c(v,g,C,U);case or:var V=C._init;return S(g,v,h,V(C._payload),U)}if(Nl(C)||vl(C))return g=g.get(h)||null,f(v,g,C,U,null);ca(v,C)}return null}function b(g,v,h,C){for(var U=null,V=null,B=v,q=v=0,F=null;B!==null&&q<h.length;q++){B.index>q?(F=B,B=null):F=B.sibling;var K=x(g,B,h[q],C);if(K===null){B===null&&(B=F);break}e&&B&&K.alternate===null&&t(g,B),v=i(K,v,q),V===null?U=K:V.sibling=K,V=K,B=F}if(q===h.length)return n(g,B),Lt&&Hr(g,q),U;if(B===null){for(;q<h.length;q++)B=u(g,h[q],C),B!==null&&(v=i(B,v,q),V===null?U=B:V.sibling=B,V=B);return Lt&&Hr(g,q),U}for(B=o(g,B);q<h.length;q++)F=S(B,g,q,h[q],C),F!==null&&(e&&F.alternate!==null&&B.delete(F.key===null?q:F.key),v=i(F,v,q),V===null?U=F:V.sibling=F,V=F);return e&&B.forEach(function(se){return t(g,se)}),Lt&&Hr(g,q),U}function N(g,v,h,C){var U=vl(h);if(typeof U!="function")throw Error(Q(150));if(h=U.call(h),h==null)throw Error(Q(151));for(var V=U=null,B=v,q=v=0,F=null,K=h.next();B!==null&&!K.done;q++,K=h.next()){B.index>q?(F=B,B=null):F=B.sibling;var se=x(g,B,K.value,C);if(se===null){B===null&&(B=F);break}e&&B&&se.alternate===null&&t(g,B),v=i(se,v,q),V===null?U=se:V.sibling=se,V=se,B=F}if(K.done)return n(g,B),Lt&&Hr(g,q),U;if(B===null){for(;!K.done;q++,K=h.next())K=u(g,K.value,C),K!==null&&(v=i(K,v,q),V===null?U=K:V.sibling=K,V=K);return Lt&&Hr(g,q),U}for(B=o(g,B);!K.done;q++,K=h.next())K=S(B,g,q,K.value,C),K!==null&&(e&&K.alternate!==null&&B.delete(K.key===null?q:K.key),v=i(K,v,q),V===null?U=K:V.sibling=K,V=K);return e&&B.forEach(function(Z){return t(g,Z)}),Lt&&Hr(g,q),U}function E(g,v,h,C){if(typeof h=="object"&&h!==null&&h.type===wi&&h.key===null&&(h=h.props.children),typeof h=="object"&&h!==null){switch(h.$$typeof){case Xs:e:{for(var U=h.key,V=v;V!==null;){if(V.key===U){if(U=h.type,U===wi){if(V.tag===7){n(g,V.sibling),v=r(V,h.props.children),v.return=g,g=v;break e}}else if(V.elementType===U||typeof U=="object"&&U!==null&&U.$$typeof===or&&ep(U)===V.type){n(g,V.sibling),v=r(V,h.props),v.ref=Cl(g,V,h),v.return=g,g=v;break e}n(g,V);break}else t(g,V);V=V.sibling}h.type===wi?(v=Gr(h.props.children,g.mode,C,h.key),v.return=g,g=v):(C=ba(h.type,h.key,h.props,null,g.mode,C),C.ref=Cl(g,v,h),C.return=g,g=C)}return l(g);case vi:e:{for(V=h.key;v!==null;){if(v.key===V)if(v.tag===4&&v.stateNode.containerInfo===h.containerInfo&&v.stateNode.implementation===h.implementation){n(g,v.sibling),v=r(v,h.children||[]),v.return=g,g=v;break e}else{n(g,v);break}else t(g,v);v=v.sibling}v=Hd(h,g.mode,C),v.return=g,g=v}return l(g);case or:return V=h._init,E(g,v,V(h._payload),C)}if(Nl(h))return b(g,v,h,C);if(vl(h))return N(g,v,h,C);ca(g,h)}return typeof h=="string"&&h!==""||typeof h=="number"?(h=""+h,v!==null&&v.tag===6?(n(g,v.sibling),v=r(v,h),v.return=g,g=v):(n(g,v),v=jd(h,g.mode,C),v.return=g,g=v),l(g)):n(g,v)}return E}var Wi=Cm(!0),Sm=Cm(!1),Ba=vr(null),za=null,Ni=null,t_=null;function n_(){t_=Ni=za=null}function o_(e){var t=Ba.current;Mt(Ba),e._currentValue=t}function xu(e,t,n){for(;e!==null;){var o=e.alternate;if((e.childLanes&t)!==t?(e.childLanes|=t,o!==null&&(o.childLanes|=t)):o!==null&&(o.childLanes&t)!==t&&(o.childLanes|=t),e===n)break;e=e.return}}function Bi(e,t){za=e,t_=Ni=null,e=e.dependencies,e!==null&&e.firstContext!==null&&((e.lanes&t)!==0&&(Sn=!0),e.firstContext=null)}function qn(e){var t=e._currentValue;if(t_!==e)if(e={context:e,memoizedValue:t,next:null},Ni===null){if(za===null)throw Error(Q(308));Ni=e,za.dependencies={lanes:0,firstContext:e}}else Ni=Ni.next=e;return t}var Qr=null;function r_(e){Qr===null?Qr=[e]:Qr.push(e)}function Mm(e,t,n,o){var r=t.interleaved;return r===null?(n.next=n,r_(t)):(n.next=r.next,r.next=n),t.interleaved=n,Vo(e,o)}function Vo(e,t){e.lanes|=t;var n=e.alternate;for(n!==null&&(n.lanes|=t),n=e,e=e.return;e!==null;)e.childLanes|=t,n=e.alternate,n!==null&&(n.childLanes|=t),n=e,e=e.return;return n.tag===3?n.stateNode:null}var rr=!1;function i_(e){e.updateQueue={baseState:e.memoizedState,firstBaseUpdate:null,lastBaseUpdate:null,shared:{pending:null,interleaved:null,lanes:0},effects:null}}function Em(e,t){e=e.updateQueue,t.updateQueue===e&&(t.updateQueue={baseState:e.baseState,firstBaseUpdate:e.firstBaseUpdate,lastBaseUpdate:e.lastBaseUpdate,shared:e.shared,effects:e.effects})}function Uo(e,t){return{eventTime:e,lane:t,tag:0,payload:null,callback:null,next:null}}function fr(e,t,n){var o=e.updateQueue;if(o===null)return null;if(o=o.shared,(Ge&2)!==0){var r=o.pending;return r===null?t.next=t:(t.next=r.next,r.next=t),o.pending=t,Vo(e,n)}return r=o.interleaved,r===null?(t.next=t,r_(o)):(t.next=r.next,r.next=t),o.interleaved=t,Vo(e,n)}function ma(e,t,n){if(t=t.updateQueue,t!==null&&(t=t.shared,(n&4194240)!==0)){var o=t.lanes;o&=e.pendingLanes,n|=o,t.lanes=n,Uu(e,n)}}function tp(e,t){var n=e.updateQueue,o=e.alternate;if(o!==null&&(o=o.updateQueue,n===o)){var r=null,i=null;if(n=n.firstBaseUpdate,n!==null){do{var l={eventTime:n.eventTime,lane:n.lane,tag:n.tag,payload:n.payload,callback:n.callback,next:null};i===null?r=i=l:i=i.next=l,n=n.next}while(n!==null);i===null?r=i=t:i=i.next=t}else r=i=t;n={baseState:o.baseState,firstBaseUpdate:r,lastBaseUpdate:i,shared:o.shared,effects:o.effects},e.updateQueue=n;return}e=n.lastBaseUpdate,e===null?n.firstBaseUpdate=t:e.next=t,n.lastBaseUpdate=t}function Oa(e,t,n,o){var r=e.updateQueue;rr=!1;var i=r.firstBaseUpdate,l=r.lastBaseUpdate,s=r.shared.pending;if(s!==null){r.shared.pending=null;var a=s,c=a.next;a.next=null,l===null?i=c:l.next=c,l=a;var f=e.alternate;f!==null&&(f=f.updateQueue,s=f.lastBaseUpdate,s!==l&&(s===null?f.firstBaseUpdate=c:s.next=c,f.lastBaseUpdate=a))}if(i!==null){var u=r.baseState;l=0,f=c=a=null,s=i;do{var x=s.lane,S=s.eventTime;if((o&x)===x){f!==null&&(f=f.next={eventTime:S,lane:0,tag:s.tag,payload:s.payload,callback:s.callback,next:null});e:{var b=e,N=s;switch(x=t,S=n,N.tag){case 1:if(b=N.payload,typeof b=="function"){u=b.call(S,u,x);break e}u=b;break e;case 3:b.flags=b.flags&-65537|128;case 0:if(b=N.payload,x=typeof b=="function"?b.call(S,u,x):b,x==null)break e;u=Rt({},u,x);break e;case 2:rr=!0}}s.callback!==null&&s.lane!==0&&(e.flags|=64,x=r.effects,x===null?r.effects=[s]:x.push(s))}else S={eventTime:S,lane:x,tag:s.tag,payload:s.payload,callback:s.callback,next:null},f===null?(c=f=S,a=u):f=f.next=S,l|=x;if(s=s.next,s===null){if(s=r.shared.pending,s===null)break;x=s,s=x.next,x.next=null,r.lastBaseUpdate=x,r.shared.pending=null}}while(!0);if(f===null&&(a=u),r.baseState=a,r.firstBaseUpdate=c,r.lastBaseUpdate=f,t=r.shared.interleaved,t!==null){r=t;do l|=r.lane,r=r.next;while(r!==t)}else i===null&&(r.shared.lanes=0);Zr|=l,e.lanes=l,e.memoizedState=u}}function np(e,t,n){if(e=t.effects,t.effects=null,e!==null)for(t=0;t<e.length;t++){var o=e[t],r=o.callback;if(r!==null){if(o.callback=null,o=n,typeof r!="function")throw Error(Q(191,r));r.call(o)}}}var ss={},Lo=vr(ss),Jl=vr(ss),Zl=vr(ss);function Vr(e){if(e===ss)throw Error(Q(174));return e}function l_(e,t){switch(wt(Zl,t),wt(Jl,e),wt(Lo,ss),e=t.nodeType,e){case 9:case 11:t=(t=t.documentElement)?t.namespaceURI:Zd(null,"");break;default:e=e===8?t.parentNode:t,t=e.namespaceURI||null,e=e.tagName,t=Zd(t,e)}Mt(Lo),wt(Lo,t)}function ji(){Mt(Lo),Mt(Jl),Mt(Zl)}function Lm(e){Vr(Zl.current);var t=Vr(Lo.current),n=Zd(t,e.type);t!==n&&(wt(Jl,e),wt(Lo,n))}function s_(e){Jl.current===e&&(Mt(Lo),Mt(Jl))}var Nt=vr(0);function Aa(e){for(var t=e;t!==null;){if(t.tag===13){var n=t.memoizedState;if(n!==null&&(n=n.dehydrated,n===null||n.data==="$?"||n.data==="$!"))return t}else if(t.tag===19&&t.memoizedProps.revealOrder!==void 0){if((t.flags&128)!==0)return t}else if(t.child!==null){t.child.return=t,t=t.child;continue}if(t===e)break;for(;t.sibling===null;){if(t.return===null||t.return===e)return null;t=t.return}t.sibling.return=t.return,t=t.sibling}return null}var Bd=[];function a_(){for(var e=0;e<Bd.length;e++)Bd[e]._workInProgressVersionPrimary=null;Bd.length=0}var ga=Go.ReactCurrentDispatcher,zd=Go.ReactCurrentBatchConfig,Jr=0,It=null,Ht=null,Qt=null,Fa=!1,zl=!1,es=0,d5=0;function dn(){throw Error(Q(321))}function c_(e,t){if(t===null)return!1;for(var n=0;n<t.length&&n<e.length;n++)if(!ho(e[n],t[n]))return!1;return!0}function d_(e,t,n,o,r,i){if(Jr=i,It=t,t.memoizedState=null,t.updateQueue=null,t.lanes=0,ga.current=e===null||e.memoizedState===null?h5:p5,e=n(o,r),zl){i=0;do{if(zl=!1,es=0,25<=i)throw Error(Q(301));i+=1,Qt=Ht=null,t.updateQueue=null,ga.current=m5,e=n(o,r)}while(zl)}if(ga.current=Wa,t=Ht!==null&&Ht.next!==null,Jr=0,Qt=Ht=It=null,Fa=!1,t)throw Error(Q(300));return e}function u_(){var e=es!==0;return es=0,e}function So(){var e={memoizedState:null,baseState:null,baseQueue:null,queue:null,next:null};return Qt===null?It.memoizedState=Qt=e:Qt=Qt.next=e,Qt}function Kn(){if(Ht===null){var e=It.alternate;e=e!==null?e.memoizedState:null}else e=Ht.next;var t=Qt===null?It.memoizedState:Qt.next;if(t!==null)Qt=t,Ht=e;else{if(e===null)throw Error(Q(310));Ht=e,e={memoizedState:Ht.memoizedState,baseState:Ht.baseState,baseQueue:Ht.baseQueue,queue:Ht.queue,next:null},Qt===null?It.memoizedState=Qt=e:Qt=Qt.next=e}return Qt}function ts(e,t){return typeof t=="function"?t(e):t}function Od(e){var t=Kn(),n=t.queue;if(n===null)throw Error(Q(311));n.lastRenderedReducer=e;var o=Ht,r=o.baseQueue,i=n.pending;if(i!==null){if(r!==null){var l=r.next;r.next=i.next,i.next=l}o.baseQueue=r=i,n.pending=null}if(r!==null){i=r.next,o=o.baseState;var s=l=null,a=null,c=i;do{var f=c.lane;if((Jr&f)===f)a!==null&&(a=a.next={lane:0,action:c.action,hasEagerState:c.hasEagerState,eagerState:c.eagerState,next:null}),o=c.hasEagerState?c.eagerState:e(o,c.action);else{var u={lane:f,action:c.action,hasEagerState:c.hasEagerState,eagerState:c.eagerState,next:null};a===null?(s=a=u,l=o):a=a.next=u,It.lanes|=f,Zr|=f}c=c.next}while(c!==null&&c!==i);a===null?l=o:a.next=s,ho(o,t.memoizedState)||(Sn=!0),t.memoizedState=o,t.baseState=l,t.baseQueue=a,n.lastRenderedState=o}if(e=n.interleaved,e!==null){r=e;do i=r.lane,It.lanes|=i,Zr|=i,r=r.next;while(r!==e)}else r===null&&(n.lanes=0);return[t.memoizedState,n.dispatch]}function Ad(e){var t=Kn(),n=t.queue;if(n===null)throw Error(Q(311));n.lastRenderedReducer=e;var o=n.dispatch,r=n.pending,i=t.memoizedState;if(r!==null){n.pending=null;var l=r=r.next;do i=e(i,l.action),l=l.next;while(l!==r);ho(i,t.memoizedState)||(Sn=!0),t.memoizedState=i,t.baseQueue===null&&(t.baseState=i),n.lastRenderedState=i}return[i,o]}function Nm(){}function Im(e,t){var n=It,o=Kn(),r=t(),i=!ho(o.memoizedState,r);if(i&&(o.memoizedState=r,Sn=!0),o=o.queue,__(Tm.bind(null,n,o,e),[e]),o.getSnapshot!==t||i||Qt!==null&&Qt.memoizedState.tag&1){if(n.flags|=2048,ns(9,$m.bind(null,n,o,r,t),void 0,null),Vt===null)throw Error(Q(349));(Jr&30)!==0||Rm(n,t,r)}return r}function Rm(e,t,n){e.flags|=16384,e={getSnapshot:t,value:n},t=It.updateQueue,t===null?(t={lastEffect:null,stores:null},It.updateQueue=t,t.stores=[e]):(n=t.stores,n===null?t.stores=[e]:n.push(e))}function $m(e,t,n,o){t.value=n,t.getSnapshot=o,Pm(t)&&Dm(e)}function Tm(e,t,n){return n(function(){Pm(t)&&Dm(e)})}function Pm(e){var t=e.getSnapshot;e=e.value;try{var n=t();return!ho(e,n)}catch{return!0}}function Dm(e){var t=Vo(e,1);t!==null&&fo(t,e,1,-1)}function op(e){var t=So();return typeof e=="function"&&(e=e()),t.memoizedState=t.baseState=e,e={pending:null,interleaved:null,lanes:0,dispatch:null,lastRenderedReducer:ts,lastRenderedState:e},t.queue=e,e=e.dispatch=f5.bind(null,It,e),[t.memoizedState,e]}function ns(e,t,n,o){return e={tag:e,create:t,destroy:n,deps:o,next:null},t=It.updateQueue,t===null?(t={lastEffect:null,stores:null},It.updateQueue=t,t.lastEffect=e.next=e):(n=t.lastEffect,n===null?t.lastEffect=e.next=e:(o=n.next,n.next=e,e.next=o,t.lastEffect=e)),e}function Bm(){return Kn().memoizedState}function ya(e,t,n,o){var r=So();It.flags|=e,r.memoizedState=ns(1|t,n,void 0,o===void 0?null:o)}function Za(e,t,n,o){var r=Kn();o=o===void 0?null:o;var i=void 0;if(Ht!==null){var l=Ht.memoizedState;if(i=l.destroy,o!==null&&c_(o,l.deps)){r.memoizedState=ns(t,n,i,o);return}}It.flags|=e,r.memoizedState=ns(1|t,n,i,o)}function rp(e,t){return ya(8390656,8,e,t)}function __(e,t){return Za(2048,8,e,t)}function zm(e,t){return Za(4,2,e,t)}function Om(e,t){return Za(4,4,e,t)}function Am(e,t){if(typeof t=="function")return e=e(),t(e),function(){t(null)};if(t!=null)return e=e(),t.current=e,function(){t.current=null}}function Fm(e,t,n){return n=n!=null?n.concat([e]):null,Za(4,4,Am.bind(null,t,e),n)}function f_(){}function Wm(e,t){var n=Kn();t=t===void 0?null:t;var o=n.memoizedState;return o!==null&&t!==null&&c_(t,o[1])?o[0]:(n.memoizedState=[e,t],e)}function jm(e,t){var n=Kn();t=t===void 0?null:t;var o=n.memoizedState;return o!==null&&t!==null&&c_(t,o[1])?o[0]:(e=e(),n.memoizedState=[e,t],e)}function Hm(e,t,n){return(Jr&21)===0?(e.baseState&&(e.baseState=!1,Sn=!0),e.memoizedState=n):(ho(n,t)||(n=Xp(),It.lanes|=n,Zr|=n,e.baseState=!0),t)}function u5(e,t){var n=dt;dt=n!==0&&4>n?n:4,e(!0);var o=zd.transition;zd.transition={};try{e(!1),t()}finally{dt=n,zd.transition=o}}function Um(){return Kn().memoizedState}function _5(e,t,n){var o=pr(e);if(n={lane:o,action:n,hasEagerState:!1,eagerState:null,next:null},Ym(e))Qm(t,n);else if(n=Mm(e,t,n,o),n!==null){var r=yn();fo(n,e,o,r),Vm(n,t,o)}}function f5(e,t,n){var o=pr(e),r={lane:o,action:n,hasEagerState:!1,eagerState:null,next:null};if(Ym(e))Qm(t,r);else{var i=e.alternate;if(e.lanes===0&&(i===null||i.lanes===0)&&(i=t.lastRenderedReducer,i!==null))try{var l=t.lastRenderedState,s=i(l,n);if(r.hasEagerState=!0,r.eagerState=s,ho(s,l)){var a=t.interleaved;a===null?(r.next=r,r_(t)):(r.next=a.next,a.next=r),t.interleaved=r;return}}catch{}n=Mm(e,t,r,o),n!==null&&(r=yn(),fo(n,e,o,r),Vm(n,t,o))}}function Ym(e){var t=e.alternate;return e===It||t!==null&&t===It}function Qm(e,t){zl=Fa=!0;var n=e.pending;n===null?t.next=t:(t.next=n.next,n.next=t),e.pending=t}function Vm(e,t,n){if((n&4194240)!==0){var o=t.lanes;o&=e.pendingLanes,n|=o,t.lanes=n,Uu(e,n)}}var Wa={readContext:qn,useCallback:dn,useContext:dn,useEffect:dn,useImperativeHandle:dn,useInsertionEffect:dn,useLayoutEffect:dn,useMemo:dn,useReducer:dn,useRef:dn,useState:dn,useDebugValue:dn,useDeferredValue:dn,useTransition:dn,useMutableSource:dn,useSyncExternalStore:dn,useId:dn,unstable_isNewReconciler:!1},h5={readContext:qn,useCallback:function(e,t){return So().memoizedState=[e,t===void 0?null:t],e},useContext:qn,useEffect:rp,useImperativeHandle:function(e,t,n){return n=n!=null?n.concat([e]):null,ya(4194308,4,Am.bind(null,t,e),n)},useLayoutEffect:function(e,t){return ya(4194308,4,e,t)},useInsertionEffect:function(e,t){return ya(4,2,e,t)},useMemo:function(e,t){var n=So();return t=t===void 0?null:t,e=e(),n.memoizedState=[e,t],e},useReducer:function(e,t,n){var o=So();return t=n!==void 0?n(t):t,o.memoizedState=o.baseState=t,e={pending:null,interleaved:null,lanes:0,dispatch:null,lastRenderedReducer:e,lastRenderedState:t},o.queue=e,e=e.dispatch=_5.bind(null,It,e),[o.memoizedState,e]},useRef:function(e){var t=So();return e={current:e},t.memoizedState=e},useState:op,useDebugValue:f_,useDeferredValue:function(e){return So().memoizedState=e},useTransition:function(){var e=op(!1),t=e[0];return e=u5.bind(null,e[1]),So().memoizedState=e,[t,e]},useMutableSource:function(){},useSyncExternalStore:function(e,t,n){var o=It,r=So();if(Lt){if(n===void 0)throw Error(Q(407));n=n()}else{if(n=t(),Vt===null)throw Error(Q(349));(Jr&30)!==0||Rm(o,t,n)}r.memoizedState=n;var i={value:n,getSnapshot:t};return r.queue=i,rp(Tm.bind(null,o,i,e),[e]),o.flags|=2048,ns(9,$m.bind(null,o,i,n,t),void 0,null),n},useId:function(){var e=So(),t=Vt.identifierPrefix;if(Lt){var n=Ho,o=jo;n=(o&~(1<<32-_o(o)-1)).toString(32)+n,t=":"+t+"R"+n,n=es++,0<n&&(t+="H"+n.toString(32)),t+=":"}else n=d5++,t=":"+t+"r"+n.toString(32)+":";return e.memoizedState=t},unstable_isNewReconciler:!1},p5={readContext:qn,useCallback:Wm,useContext:qn,useEffect:__,useImperativeHandle:Fm,useInsertionEffect:zm,useLayoutEffect:Om,useMemo:jm,useReducer:Od,useRef:Bm,useState:function(){return Od(ts)},useDebugValue:f_,useDeferredValue:function(e){var t=Kn();return Hm(t,Ht.memoizedState,e)},useTransition:function(){var e=Od(ts)[0],t=Kn().memoizedState;return[e,t]},useMutableSource:Nm,useSyncExternalStore:Im,useId:Um,unstable_isNewReconciler:!1},m5={readContext:qn,useCallback:Wm,useContext:qn,useEffect:__,useImperativeHandle:Fm,useInsertionEffect:zm,useLayoutEffect:Om,useMemo:jm,useReducer:Ad,useRef:Bm,useState:function(){return Ad(ts)},useDebugValue:f_,useDeferredValue:function(e){var t=Kn();return Ht===null?t.memoizedState=e:Hm(t,Ht.memoizedState,e)},useTransition:function(){var e=Ad(ts)[0],t=Kn().memoizedState;return[e,t]},useMutableSource:Nm,useSyncExternalStore:Im,useId:Um,unstable_isNewReconciler:!1};function ao(e,t){if(e&&e.defaultProps){t=Rt({},t),e=e.defaultProps;for(var n in e)t[n]===void 0&&(t[n]=e[n]);return t}return t}function vu(e,t,n,o){t=e.memoizedState,n=n(o,t),n=n==null?t:Rt({},t,n),e.memoizedState=n,e.lanes===0&&(e.updateQueue.baseState=n)}var ec={isMounted:function(e){return(e=e._reactInternals)?ni(e)===e:!1},enqueueSetState:function(e,t,n){e=e._reactInternals;var o=yn(),r=pr(e),i=Uo(o,r);i.payload=t,n!=null&&(i.callback=n),t=fr(e,i,r),t!==null&&(fo(t,e,r,o),ma(t,e,r))},enqueueReplaceState:function(e,t,n){e=e._reactInternals;var o=yn(),r=pr(e),i=Uo(o,r);i.tag=1,i.payload=t,n!=null&&(i.callback=n),t=fr(e,i,r),t!==null&&(fo(t,e,r,o),ma(t,e,r))},enqueueForceUpdate:function(e,t){e=e._reactInternals;var n=yn(),o=pr(e),r=Uo(n,o);r.tag=2,t!=null&&(r.callback=t),t=fr(e,r,o),t!==null&&(fo(t,e,o,n),ma(t,e,o))}};function ip(e,t,n,o,r,i,l){return e=e.stateNode,typeof e.shouldComponentUpdate=="function"?e.shouldComponentUpdate(o,i,l):t.prototype&&t.prototype.isPureReactComponent?!Xl(n,o)||!Xl(r,i):!0}function Xm(e,t,n){var o=!1,r=yr,i=t.contextType;return typeof i=="object"&&i!==null?i=qn(i):(r=En(t)?qr:fn.current,o=t.contextTypes,i=(o=o!=null)?Ai(e,r):yr),t=new t(n,i),e.memoizedState=t.state!==null&&t.state!==void 0?t.state:null,t.updater=ec,e.stateNode=t,t._reactInternals=e,o&&(e=e.stateNode,e.__reactInternalMemoizedUnmaskedChildContext=r,e.__reactInternalMemoizedMaskedChildContext=i),t}function lp(e,t,n,o){e=t.state,typeof t.componentWillReceiveProps=="function"&&t.componentWillReceiveProps(n,o),typeof t.UNSAFE_componentWillReceiveProps=="function"&&t.UNSAFE_componentWillReceiveProps(n,o),t.state!==e&&ec.enqueueReplaceState(t,t.state,null)}function wu(e,t,n,o){var r=e.stateNode;r.props=n,r.state=e.memoizedState,r.refs={},i_(e);var i=t.contextType;typeof i=="object"&&i!==null?r.context=qn(i):(i=En(t)?qr:fn.current,r.context=Ai(e,i)),r.state=e.memoizedState,i=t.getDerivedStateFromProps,typeof i=="function"&&(vu(e,t,i,n),r.state=e.memoizedState),typeof t.getDerivedStateFromProps=="function"||typeof r.getSnapshotBeforeUpdate=="function"||typeof r.UNSAFE_componentWillMount!="function"&&typeof r.componentWillMount!="function"||(t=r.state,typeof r.componentWillMount=="function"&&r.componentWillMount(),typeof r.UNSAFE_componentWillMount=="function"&&r.UNSAFE_componentWillMount(),t!==r.state&&ec.enqueueReplaceState(r,r.state,null),Oa(e,n,r,o),r.state=e.memoizedState),typeof r.componentDidMount=="function"&&(e.flags|=4194308)}function Hi(e,t){try{var n="",o=t;do n+=Q1(o),o=o.return;while(o);var r=n}catch(i){r=`
Error generating stack: `+i.message+`
`+i.stack}return{value:e,source:t,stack:r,digest:null}}function Fd(e,t,n){return{value:e,source:null,stack:n??null,digest:t??null}}function bu(e,t){try{console.error(t.value)}catch(n){setTimeout(function(){throw n})}}var g5=typeof WeakMap=="function"?WeakMap:Map;function Gm(e,t,n){n=Uo(-1,n),n.tag=3,n.payload={element:null};var o=t.value;return n.callback=function(){Ha||(Ha=!0,$u=o),bu(e,t)},n}function qm(e,t,n){n=Uo(-1,n),n.tag=3;var o=e.type.getDerivedStateFromError;if(typeof o=="function"){var r=t.value;n.payload=function(){return o(r)},n.callback=function(){bu(e,t)}}var i=e.stateNode;return i!==null&&typeof i.componentDidCatch=="function"&&(n.callback=function(){bu(e,t),typeof o!="function"&&(hr===null?hr=new Set([this]):hr.add(this));var l=t.stack;this.componentDidCatch(t.value,{componentStack:l!==null?l:""})}),n}function sp(e,t,n){var o=e.pingCache;if(o===null){o=e.pingCache=new g5;var r=new Set;o.set(t,r)}else r=o.get(t),r===void 0&&(r=new Set,o.set(t,r));r.has(n)||(r.add(n),e=R5.bind(null,e,t,n),t.then(e,e))}function ap(e){do{var t;if((t=e.tag===13)&&(t=e.memoizedState,t=t!==null?t.dehydrated!==null:!0),t)return e;e=e.return}while(e!==null);return null}function cp(e,t,n,o,r){return(e.mode&1)===0?(e===t?e.flags|=65536:(e.flags|=128,n.flags|=131072,n.flags&=-52805,n.tag===1&&(n.alternate===null?n.tag=17:(t=Uo(-1,1),t.tag=2,fr(n,t,1))),n.lanes|=1),e):(e.flags|=65536,e.lanes=r,e)}var y5=Go.ReactCurrentOwner,Sn=!1;function gn(e,t,n,o){t.child=e===null?Sm(t,null,n,o):Wi(t,e.child,n,o)}function dp(e,t,n,o,r){n=n.render;var i=t.ref;return Bi(t,r),o=d_(e,t,n,o,i,r),n=u_(),e!==null&&!Sn?(t.updateQueue=e.updateQueue,t.flags&=-2053,e.lanes&=~r,Xo(e,t,r)):(Lt&&n&&Ju(t),t.flags|=1,gn(e,t,o,r),t.child)}function up(e,t,n,o,r){if(e===null){var i=n.type;return typeof i=="function"&&!w_(i)&&i.defaultProps===void 0&&n.compare===null&&n.defaultProps===void 0?(t.tag=15,t.type=i,Km(e,t,i,o,r)):(e=ba(n.type,null,o,t,t.mode,r),e.ref=t.ref,e.return=t,t.child=e)}if(i=e.child,(e.lanes&r)===0){var l=i.memoizedProps;if(n=n.compare,n=n!==null?n:Xl,n(l,o)&&e.ref===t.ref)return Xo(e,t,r)}return t.flags|=1,e=mr(i,o),e.ref=t.ref,e.return=t,t.child=e}function Km(e,t,n,o,r){if(e!==null){var i=e.memoizedProps;if(Xl(i,o)&&e.ref===t.ref)if(Sn=!1,t.pendingProps=o=i,(e.lanes&r)!==0)(e.flags&131072)!==0&&(Sn=!0);else return t.lanes=e.lanes,Xo(e,t,r)}return ku(e,t,n,o,r)}function Jm(e,t,n){var o=t.pendingProps,r=o.children,i=e!==null?e.memoizedState:null;if(o.mode==="hidden")if((t.mode&1)===0)t.memoizedState={baseLanes:0,cachePool:null,transitions:null},wt(Ri,Rn),Rn|=n;else{if((n&1073741824)===0)return e=i!==null?i.baseLanes|n:n,t.lanes=t.childLanes=1073741824,t.memoizedState={baseLanes:e,cachePool:null,transitions:null},t.updateQueue=null,wt(Ri,Rn),Rn|=e,null;t.memoizedState={baseLanes:0,cachePool:null,transitions:null},o=i!==null?i.baseLanes:n,wt(Ri,Rn),Rn|=o}else i!==null?(o=i.baseLanes|n,t.memoizedState=null):o=n,wt(Ri,Rn),Rn|=o;return gn(e,t,r,n),t.child}function Zm(e,t){var n=t.ref;(e===null&&n!==null||e!==null&&e.ref!==n)&&(t.flags|=512,t.flags|=2097152)}function ku(e,t,n,o,r){var i=En(n)?qr:fn.current;return i=Ai(t,i),Bi(t,r),n=d_(e,t,n,o,i,r),o=u_(),e!==null&&!Sn?(t.updateQueue=e.updateQueue,t.flags&=-2053,e.lanes&=~r,Xo(e,t,r)):(Lt&&o&&Ju(t),t.flags|=1,gn(e,t,n,r),t.child)}function _p(e,t,n,o,r){if(En(n)){var i=!0;Ta(t)}else i=!1;if(Bi(t,r),t.stateNode===null)xa(e,t),Xm(t,n,o),wu(t,n,o,r),o=!0;else if(e===null){var l=t.stateNode,s=t.memoizedProps;l.props=s;var a=l.context,c=n.contextType;typeof c=="object"&&c!==null?c=qn(c):(c=En(n)?qr:fn.current,c=Ai(t,c));var f=n.getDerivedStateFromProps,u=typeof f=="function"||typeof l.getSnapshotBeforeUpdate=="function";u||typeof l.UNSAFE_componentWillReceiveProps!="function"&&typeof l.componentWillReceiveProps!="function"||(s!==o||a!==c)&&lp(t,l,o,c),rr=!1;var x=t.memoizedState;l.state=x,Oa(t,o,l,r),a=t.memoizedState,s!==o||x!==a||Mn.current||rr?(typeof f=="function"&&(vu(t,n,f,o),a=t.memoizedState),(s=rr||ip(t,n,s,o,x,a,c))?(u||typeof l.UNSAFE_componentWillMount!="function"&&typeof l.componentWillMount!="function"||(typeof l.componentWillMount=="function"&&l.componentWillMount(),typeof l.UNSAFE_componentWillMount=="function"&&l.UNSAFE_componentWillMount()),typeof l.componentDidMount=="function"&&(t.flags|=4194308)):(typeof l.componentDidMount=="function"&&(t.flags|=4194308),t.memoizedProps=o,t.memoizedState=a),l.props=o,l.state=a,l.context=c,o=s):(typeof l.componentDidMount=="function"&&(t.flags|=4194308),o=!1)}else{l=t.stateNode,Em(e,t),s=t.memoizedProps,c=t.type===t.elementType?s:ao(t.type,s),l.props=c,u=t.pendingProps,x=l.context,a=n.contextType,typeof a=="object"&&a!==null?a=qn(a):(a=En(n)?qr:fn.current,a=Ai(t,a));var S=n.getDerivedStateFromProps;(f=typeof S=="function"||typeof l.getSnapshotBeforeUpdate=="function")||typeof l.UNSAFE_componentWillReceiveProps!="function"&&typeof l.componentWillReceiveProps!="function"||(s!==u||x!==a)&&lp(t,l,o,a),rr=!1,x=t.memoizedState,l.state=x,Oa(t,o,l,r);var b=t.memoizedState;s!==u||x!==b||Mn.current||rr?(typeof S=="function"&&(vu(t,n,S,o),b=t.memoizedState),(c=rr||ip(t,n,c,o,x,b,a)||!1)?(f||typeof l.UNSAFE_componentWillUpdate!="function"&&typeof l.componentWillUpdate!="function"||(typeof l.componentWillUpdate=="function"&&l.componentWillUpdate(o,b,a),typeof l.UNSAFE_componentWillUpdate=="function"&&l.UNSAFE_componentWillUpdate(o,b,a)),typeof l.componentDidUpdate=="function"&&(t.flags|=4),typeof l.getSnapshotBeforeUpdate=="function"&&(t.flags|=1024)):(typeof l.componentDidUpdate!="function"||s===e.memoizedProps&&x===e.memoizedState||(t.flags|=4),typeof l.getSnapshotBeforeUpdate!="function"||s===e.memoizedProps&&x===e.memoizedState||(t.flags|=1024),t.memoizedProps=o,t.memoizedState=b),l.props=o,l.state=b,l.context=a,o=c):(typeof l.componentDidUpdate!="function"||s===e.memoizedProps&&x===e.memoizedState||(t.flags|=4),typeof l.getSnapshotBeforeUpdate!="function"||s===e.memoizedProps&&x===e.memoizedState||(t.flags|=1024),o=!1)}return Cu(e,t,n,o,i,r)}function Cu(e,t,n,o,r,i){Zm(e,t);var l=(t.flags&128)!==0;if(!o&&!l)return r&&Kh(t,n,!1),Xo(e,t,i);o=t.stateNode,y5.current=t;var s=l&&typeof n.getDerivedStateFromError!="function"?null:o.render();return t.flags|=1,e!==null&&l?(t.child=Wi(t,e.child,null,i),t.child=Wi(t,null,s,i)):gn(e,t,s,i),t.memoizedState=o.state,r&&Kh(t,n,!0),t.child}function e0(e){var t=e.stateNode;t.pendingContext?qh(e,t.pendingContext,t.pendingContext!==t.context):t.context&&qh(e,t.context,!1),l_(e,t.containerInfo)}function fp(e,t,n,o,r){return Fi(),e_(r),t.flags|=256,gn(e,t,n,o),t.child}var Su={dehydrated:null,treeContext:null,retryLane:0};function Mu(e){return{baseLanes:e,cachePool:null,transitions:null}}function t0(e,t,n){var o=t.pendingProps,r=Nt.current,i=!1,l=(t.flags&128)!==0,s;if((s=l)||(s=e!==null&&e.memoizedState===null?!1:(r&2)!==0),s?(i=!0,t.flags&=-129):(e===null||e.memoizedState!==null)&&(r|=1),wt(Nt,r&1),e===null)return yu(t),e=t.memoizedState,e!==null&&(e=e.dehydrated,e!==null)?((t.mode&1)===0?t.lanes=1:e.data==="$!"?t.lanes=8:t.lanes=1073741824,null):(l=o.children,e=o.fallback,i?(o=t.mode,i=t.child,l={mode:"hidden",children:l},(o&1)===0&&i!==null?(i.childLanes=0,i.pendingProps=l):i=oc(l,o,0,null),e=Gr(e,o,n,null),i.return=t,e.return=t,i.sibling=e,t.child=i,t.child.memoizedState=Mu(n),t.memoizedState=Su,e):h_(t,l));if(r=e.memoizedState,r!==null&&(s=r.dehydrated,s!==null))return x5(e,t,l,o,s,r,n);if(i){i=o.fallback,l=t.mode,r=e.child,s=r.sibling;var a={mode:"hidden",children:o.children};return(l&1)===0&&t.child!==r?(o=t.child,o.childLanes=0,o.pendingProps=a,t.deletions=null):(o=mr(r,a),o.subtreeFlags=r.subtreeFlags&14680064),s!==null?i=mr(s,i):(i=Gr(i,l,n,null),i.flags|=2),i.return=t,o.return=t,o.sibling=i,t.child=o,o=i,i=t.child,l=e.child.memoizedState,l=l===null?Mu(n):{baseLanes:l.baseLanes|n,cachePool:null,transitions:l.transitions},i.memoizedState=l,i.childLanes=e.childLanes&~n,t.memoizedState=Su,o}return i=e.child,e=i.sibling,o=mr(i,{mode:"visible",children:o.children}),(t.mode&1)===0&&(o.lanes=n),o.return=t,o.sibling=null,e!==null&&(n=t.deletions,n===null?(t.deletions=[e],t.flags|=16):n.push(e)),t.child=o,t.memoizedState=null,o}function h_(e,t){return t=oc({mode:"visible",children:t},e.mode,0,null),t.return=e,e.child=t}function da(e,t,n,o){return o!==null&&e_(o),Wi(t,e.child,null,n),e=h_(t,t.pendingProps.children),e.flags|=2,t.memoizedState=null,e}function x5(e,t,n,o,r,i,l){if(n)return t.flags&256?(t.flags&=-257,o=Fd(Error(Q(422))),da(e,t,l,o)):t.memoizedState!==null?(t.child=e.child,t.flags|=128,null):(i=o.fallback,r=t.mode,o=oc({mode:"visible",children:o.children},r,0,null),i=Gr(i,r,l,null),i.flags|=2,o.return=t,i.return=t,o.sibling=i,t.child=o,(t.mode&1)!==0&&Wi(t,e.child,null,l),t.child.memoizedState=Mu(l),t.memoizedState=Su,i);if((t.mode&1)===0)return da(e,t,l,null);if(r.data==="$!"){if(o=r.nextSibling&&r.nextSibling.dataset,o)var s=o.dgst;return o=s,i=Error(Q(419)),o=Fd(i,o,void 0),da(e,t,l,o)}if(s=(l&e.childLanes)!==0,Sn||s){if(o=Vt,o!==null){switch(l&-l){case 4:r=2;break;case 16:r=8;break;case 64:case 128:case 256:case 512:case 1024:case 2048:case 4096:case 8192:case 16384:case 32768:case 65536:case 131072:case 262144:case 524288:case 1048576:case 2097152:case 4194304:case 8388608:case 16777216:case 33554432:case 67108864:r=32;break;case 536870912:r=268435456;break;default:r=0}r=(r&(o.suspendedLanes|l))!==0?0:r,r!==0&&r!==i.retryLane&&(i.retryLane=r,Vo(e,r),fo(o,e,r,-1))}return v_(),o=Fd(Error(Q(421))),da(e,t,l,o)}return r.data==="$?"?(t.flags|=128,t.child=e.child,t=$5.bind(null,e),r._reactRetry=t,null):(e=i.treeContext,$n=_r(r.nextSibling),Tn=t,Lt=!0,uo=null,e!==null&&(Qn[Vn++]=jo,Qn[Vn++]=Ho,Qn[Vn++]=Kr,jo=e.id,Ho=e.overflow,Kr=t),t=h_(t,o.children),t.flags|=4096,t)}function hp(e,t,n){e.lanes|=t;var o=e.alternate;o!==null&&(o.lanes|=t),xu(e.return,t,n)}function Wd(e,t,n,o,r){var i=e.memoizedState;i===null?e.memoizedState={isBackwards:t,rendering:null,renderingStartTime:0,last:o,tail:n,tailMode:r}:(i.isBackwards=t,i.rendering=null,i.renderingStartTime=0,i.last=o,i.tail=n,i.tailMode=r)}function n0(e,t,n){var o=t.pendingProps,r=o.revealOrder,i=o.tail;if(gn(e,t,o.children,n),o=Nt.current,(o&2)!==0)o=o&1|2,t.flags|=128;else{if(e!==null&&(e.flags&128)!==0)e:for(e=t.child;e!==null;){if(e.tag===13)e.memoizedState!==null&&hp(e,n,t);else if(e.tag===19)hp(e,n,t);else if(e.child!==null){e.child.return=e,e=e.child;continue}if(e===t)break e;for(;e.sibling===null;){if(e.return===null||e.return===t)break e;e=e.return}e.sibling.return=e.return,e=e.sibling}o&=1}if(wt(Nt,o),(t.mode&1)===0)t.memoizedState=null;else switch(r){case"forwards":for(n=t.child,r=null;n!==null;)e=n.alternate,e!==null&&Aa(e)===null&&(r=n),n=n.sibling;n=r,n===null?(r=t.child,t.child=null):(r=n.sibling,n.sibling=null),Wd(t,!1,r,n,i);break;case"backwards":for(n=null,r=t.child,t.child=null;r!==null;){if(e=r.alternate,e!==null&&Aa(e)===null){t.child=r;break}e=r.sibling,r.sibling=n,n=r,r=e}Wd(t,!0,n,null,i);break;case"together":Wd(t,!1,null,null,void 0);break;default:t.memoizedState=null}return t.child}function xa(e,t){(t.mode&1)===0&&e!==null&&(e.alternate=null,t.alternate=null,t.flags|=2)}function Xo(e,t,n){if(e!==null&&(t.dependencies=e.dependencies),Zr|=t.lanes,(n&t.childLanes)===0)return null;if(e!==null&&t.child!==e.child)throw Error(Q(153));if(t.child!==null){for(e=t.child,n=mr(e,e.pendingProps),t.child=n,n.return=t;e.sibling!==null;)e=e.sibling,n=n.sibling=mr(e,e.pendingProps),n.return=t;n.sibling=null}return t.child}function v5(e,t,n){switch(t.tag){case 3:e0(t),Fi();break;case 5:Lm(t);break;case 1:En(t.type)&&Ta(t);break;case 4:l_(t,t.stateNode.containerInfo);break;case 10:var o=t.type._context,r=t.memoizedProps.value;wt(Ba,o._currentValue),o._currentValue=r;break;case 13:if(o=t.memoizedState,o!==null)return o.dehydrated!==null?(wt(Nt,Nt.current&1),t.flags|=128,null):(n&t.child.childLanes)!==0?t0(e,t,n):(wt(Nt,Nt.current&1),e=Xo(e,t,n),e!==null?e.sibling:null);wt(Nt,Nt.current&1);break;case 19:if(o=(n&t.childLanes)!==0,(e.flags&128)!==0){if(o)return n0(e,t,n);t.flags|=128}if(r=t.memoizedState,r!==null&&(r.rendering=null,r.tail=null,r.lastEffect=null),wt(Nt,Nt.current),o)break;return null;case 22:case 23:return t.lanes=0,Jm(e,t,n)}return Xo(e,t,n)}var o0,Eu,r0,i0;o0=function(e,t){for(var n=t.child;n!==null;){if(n.tag===5||n.tag===6)e.appendChild(n.stateNode);else if(n.tag!==4&&n.child!==null){n.child.return=n,n=n.child;continue}if(n===t)break;for(;n.sibling===null;){if(n.return===null||n.return===t)return;n=n.return}n.sibling.return=n.return,n=n.sibling}};Eu=function(){};r0=function(e,t,n,o){var r=e.memoizedProps;if(r!==o){e=t.stateNode,Vr(Lo.current);var i=null;switch(n){case"input":r=Gd(e,r),o=Gd(e,o),i=[];break;case"select":r=Rt({},r,{value:void 0}),o=Rt({},o,{value:void 0}),i=[];break;case"textarea":r=Jd(e,r),o=Jd(e,o),i=[];break;default:typeof r.onClick!="function"&&typeof o.onClick=="function"&&(e.onclick=Ra)}eu(n,o);var l;n=null;for(c in r)if(!o.hasOwnProperty(c)&&r.hasOwnProperty(c)&&r[c]!=null)if(c==="style"){var s=r[c];for(l in s)s.hasOwnProperty(l)&&(n||(n={}),n[l]="")}else c!=="dangerouslySetInnerHTML"&&c!=="children"&&c!=="suppressContentEditableWarning"&&c!=="suppressHydrationWarning"&&c!=="autoFocus"&&(Wl.hasOwnProperty(c)?i||(i=[]):(i=i||[]).push(c,null));for(c in o){var a=o[c];if(s=r?.[c],o.hasOwnProperty(c)&&a!==s&&(a!=null||s!=null))if(c==="style")if(s){for(l in s)!s.hasOwnProperty(l)||a&&a.hasOwnProperty(l)||(n||(n={}),n[l]="");for(l in a)a.hasOwnProperty(l)&&s[l]!==a[l]&&(n||(n={}),n[l]=a[l])}else n||(i||(i=[]),i.push(c,n)),n=a;else c==="dangerouslySetInnerHTML"?(a=a?a.__html:void 0,s=s?s.__html:void 0,a!=null&&s!==a&&(i=i||[]).push(c,a)):c==="children"?typeof a!="string"&&typeof a!="number"||(i=i||[]).push(c,""+a):c!=="suppressContentEditableWarning"&&c!=="suppressHydrationWarning"&&(Wl.hasOwnProperty(c)?(a!=null&&c==="onScroll"&&St("scroll",e),i||s===a||(i=[])):(i=i||[]).push(c,a))}n&&(i=i||[]).push("style",n);var c=i;(t.updateQueue=c)&&(t.flags|=4)}};i0=function(e,t,n,o){n!==o&&(t.flags|=4)};function Sl(e,t){if(!Lt)switch(e.tailMode){case"hidden":t=e.tail;for(var n=null;t!==null;)t.alternate!==null&&(n=t),t=t.sibling;n===null?e.tail=null:n.sibling=null;break;case"collapsed":n=e.tail;for(var o=null;n!==null;)n.alternate!==null&&(o=n),n=n.sibling;o===null?t||e.tail===null?e.tail=null:e.tail.sibling=null:o.sibling=null}}function un(e){var t=e.alternate!==null&&e.alternate.child===e.child,n=0,o=0;if(t)for(var r=e.child;r!==null;)n|=r.lanes|r.childLanes,o|=r.subtreeFlags&14680064,o|=r.flags&14680064,r.return=e,r=r.sibling;else for(r=e.child;r!==null;)n|=r.lanes|r.childLanes,o|=r.subtreeFlags,o|=r.flags,r.return=e,r=r.sibling;return e.subtreeFlags|=o,e.childLanes=n,t}function w5(e,t,n){var o=t.pendingProps;switch(Zu(t),t.tag){case 2:case 16:case 15:case 0:case 11:case 7:case 8:case 12:case 9:case 14:return un(t),null;case 1:return En(t.type)&&$a(),un(t),null;case 3:return o=t.stateNode,ji(),Mt(Mn),Mt(fn),a_(),o.pendingContext&&(o.context=o.pendingContext,o.pendingContext=null),(e===null||e.child===null)&&(aa(t)?t.flags|=4:e===null||e.memoizedState.isDehydrated&&(t.flags&256)===0||(t.flags|=1024,uo!==null&&(Du(uo),uo=null))),Eu(e,t),un(t),null;case 5:s_(t);var r=Vr(Zl.current);if(n=t.type,e!==null&&t.stateNode!=null)r0(e,t,n,o,r),e.ref!==t.ref&&(t.flags|=512,t.flags|=2097152);else{if(!o){if(t.stateNode===null)throw Error(Q(166));return un(t),null}if(e=Vr(Lo.current),aa(t)){o=t.stateNode,n=t.type;var i=t.memoizedProps;switch(o[Mo]=t,o[Kl]=i,e=(t.mode&1)!==0,n){case"dialog":St("cancel",o),St("close",o);break;case"iframe":case"object":case"embed":St("load",o);break;case"video":case"audio":for(r=0;r<Rl.length;r++)St(Rl[r],o);break;case"source":St("error",o);break;case"img":case"image":case"link":St("error",o),St("load",o);break;case"details":St("toggle",o);break;case"input":bh(o,i),St("invalid",o);break;case"select":o._wrapperState={wasMultiple:!!i.multiple},St("invalid",o);break;case"textarea":Ch(o,i),St("invalid",o)}eu(n,i),r=null;for(var l in i)if(i.hasOwnProperty(l)){var s=i[l];l==="children"?typeof s=="string"?o.textContent!==s&&(i.suppressHydrationWarning!==!0&&sa(o.textContent,s,e),r=["children",s]):typeof s=="number"&&o.textContent!==""+s&&(i.suppressHydrationWarning!==!0&&sa(o.textContent,s,e),r=["children",""+s]):Wl.hasOwnProperty(l)&&s!=null&&l==="onScroll"&&St("scroll",o)}switch(n){case"input":Gs(o),kh(o,i,!0);break;case"textarea":Gs(o),Sh(o);break;case"select":case"option":break;default:typeof i.onClick=="function"&&(o.onclick=Ra)}o=r,t.updateQueue=o,o!==null&&(t.flags|=4)}else{l=r.nodeType===9?r:r.ownerDocument,e==="http://www.w3.org/1999/xhtml"&&(e=Tp(n)),e==="http://www.w3.org/1999/xhtml"?n==="script"?(e=l.createElement("div"),e.innerHTML="<script><\/script>",e=e.removeChild(e.firstChild)):typeof o.is=="string"?e=l.createElement(n,{is:o.is}):(e=l.createElement(n),n==="select"&&(l=e,o.multiple?l.multiple=!0:o.size&&(l.size=o.size))):e=l.createElementNS(e,n),e[Mo]=t,e[Kl]=o,o0(e,t,!1,!1),t.stateNode=e;e:{switch(l=tu(n,o),n){case"dialog":St("cancel",e),St("close",e),r=o;break;case"iframe":case"object":case"embed":St("load",e),r=o;break;case"video":case"audio":for(r=0;r<Rl.length;r++)St(Rl[r],e);r=o;break;case"source":St("error",e),r=o;break;case"img":case"image":case"link":St("error",e),St("load",e),r=o;break;case"details":St("toggle",e),r=o;break;case"input":bh(e,o),r=Gd(e,o),St("invalid",e);break;case"option":r=o;break;case"select":e._wrapperState={wasMultiple:!!o.multiple},r=Rt({},o,{value:void 0}),St("invalid",e);break;case"textarea":Ch(e,o),r=Jd(e,o),St("invalid",e);break;default:r=o}eu(n,r),s=r;for(i in s)if(s.hasOwnProperty(i)){var a=s[i];i==="style"?Bp(e,a):i==="dangerouslySetInnerHTML"?(a=a?a.__html:void 0,a!=null&&Pp(e,a)):i==="children"?typeof a=="string"?(n!=="textarea"||a!=="")&&jl(e,a):typeof a=="number"&&jl(e,""+a):i!=="suppressContentEditableWarning"&&i!=="suppressHydrationWarning"&&i!=="autoFocus"&&(Wl.hasOwnProperty(i)?a!=null&&i==="onScroll"&&St("scroll",e):a!=null&&Ou(e,i,a,l))}switch(n){case"input":Gs(e),kh(e,o,!1);break;case"textarea":Gs(e),Sh(e);break;case"option":o.value!=null&&e.setAttribute("value",""+gr(o.value));break;case"select":e.multiple=!!o.multiple,i=o.value,i!=null?$i(e,!!o.multiple,i,!1):o.defaultValue!=null&&$i(e,!!o.multiple,o.defaultValue,!0);break;default:typeof r.onClick=="function"&&(e.onclick=Ra)}switch(n){case"button":case"input":case"select":case"textarea":o=!!o.autoFocus;break e;case"img":o=!0;break e;default:o=!1}}o&&(t.flags|=4)}t.ref!==null&&(t.flags|=512,t.flags|=2097152)}return un(t),null;case 6:if(e&&t.stateNode!=null)i0(e,t,e.memoizedProps,o);else{if(typeof o!="string"&&t.stateNode===null)throw Error(Q(166));if(n=Vr(Zl.current),Vr(Lo.current),aa(t)){if(o=t.stateNode,n=t.memoizedProps,o[Mo]=t,(i=o.nodeValue!==n)&&(e=Tn,e!==null))switch(e.tag){case 3:sa(o.nodeValue,n,(e.mode&1)!==0);break;case 5:e.memoizedProps.suppressHydrationWarning!==!0&&sa(o.nodeValue,n,(e.mode&1)!==0)}i&&(t.flags|=4)}else o=(n.nodeType===9?n:n.ownerDocument).createTextNode(o),o[Mo]=t,t.stateNode=o}return un(t),null;case 13:if(Mt(Nt),o=t.memoizedState,e===null||e.memoizedState!==null&&e.memoizedState.dehydrated!==null){if(Lt&&$n!==null&&(t.mode&1)!==0&&(t.flags&128)===0)km(),Fi(),t.flags|=98560,i=!1;else if(i=aa(t),o!==null&&o.dehydrated!==null){if(e===null){if(!i)throw Error(Q(318));if(i=t.memoizedState,i=i!==null?i.dehydrated:null,!i)throw Error(Q(317));i[Mo]=t}else Fi(),(t.flags&128)===0&&(t.memoizedState=null),t.flags|=4;un(t),i=!1}else uo!==null&&(Du(uo),uo=null),i=!0;if(!i)return t.flags&65536?t:null}return(t.flags&128)!==0?(t.lanes=n,t):(o=o!==null,o!==(e!==null&&e.memoizedState!==null)&&o&&(t.child.flags|=8192,(t.mode&1)!==0&&(e===null||(Nt.current&1)!==0?Ut===0&&(Ut=3):v_())),t.updateQueue!==null&&(t.flags|=4),un(t),null);case 4:return ji(),Eu(e,t),e===null&&Gl(t.stateNode.containerInfo),un(t),null;case 10:return o_(t.type._context),un(t),null;case 17:return En(t.type)&&$a(),un(t),null;case 19:if(Mt(Nt),i=t.memoizedState,i===null)return un(t),null;if(o=(t.flags&128)!==0,l=i.rendering,l===null)if(o)Sl(i,!1);else{if(Ut!==0||e!==null&&(e.flags&128)!==0)for(e=t.child;e!==null;){if(l=Aa(e),l!==null){for(t.flags|=128,Sl(i,!1),o=l.updateQueue,o!==null&&(t.updateQueue=o,t.flags|=4),t.subtreeFlags=0,o=n,n=t.child;n!==null;)i=n,e=o,i.flags&=14680066,l=i.alternate,l===null?(i.childLanes=0,i.lanes=e,i.child=null,i.subtreeFlags=0,i.memoizedProps=null,i.memoizedState=null,i.updateQueue=null,i.dependencies=null,i.stateNode=null):(i.childLanes=l.childLanes,i.lanes=l.lanes,i.child=l.child,i.subtreeFlags=0,i.deletions=null,i.memoizedProps=l.memoizedProps,i.memoizedState=l.memoizedState,i.updateQueue=l.updateQueue,i.type=l.type,e=l.dependencies,i.dependencies=e===null?null:{lanes:e.lanes,firstContext:e.firstContext}),n=n.sibling;return wt(Nt,Nt.current&1|2),t.child}e=e.sibling}i.tail!==null&&zt()>Ui&&(t.flags|=128,o=!0,Sl(i,!1),t.lanes=4194304)}else{if(!o)if(e=Aa(l),e!==null){if(t.flags|=128,o=!0,n=e.updateQueue,n!==null&&(t.updateQueue=n,t.flags|=4),Sl(i,!0),i.tail===null&&i.tailMode==="hidden"&&!l.alternate&&!Lt)return un(t),null}else 2*zt()-i.renderingStartTime>Ui&&n!==1073741824&&(t.flags|=128,o=!0,Sl(i,!1),t.lanes=4194304);i.isBackwards?(l.sibling=t.child,t.child=l):(n=i.last,n!==null?n.sibling=l:t.child=l,i.last=l)}return i.tail!==null?(t=i.tail,i.rendering=t,i.tail=t.sibling,i.renderingStartTime=zt(),t.sibling=null,n=Nt.current,wt(Nt,o?n&1|2:n&1),t):(un(t),null);case 22:case 23:return x_(),o=t.memoizedState!==null,e!==null&&e.memoizedState!==null!==o&&(t.flags|=8192),o&&(t.mode&1)!==0?(Rn&1073741824)!==0&&(un(t),t.subtreeFlags&6&&(t.flags|=8192)):un(t),null;case 24:return null;case 25:return null}throw Error(Q(156,t.tag))}function b5(e,t){switch(Zu(t),t.tag){case 1:return En(t.type)&&$a(),e=t.flags,e&65536?(t.flags=e&-65537|128,t):null;case 3:return ji(),Mt(Mn),Mt(fn),a_(),e=t.flags,(e&65536)!==0&&(e&128)===0?(t.flags=e&-65537|128,t):null;case 5:return s_(t),null;case 13:if(Mt(Nt),e=t.memoizedState,e!==null&&e.dehydrated!==null){if(t.alternate===null)throw Error(Q(340));Fi()}return e=t.flags,e&65536?(t.flags=e&-65537|128,t):null;case 19:return Mt(Nt),null;case 4:return ji(),null;case 10:return o_(t.type._context),null;case 22:case 23:return x_(),null;case 24:return null;default:return null}}var ua=!1,_n=!1,k5=typeof WeakSet=="function"?WeakSet:Set,re=null;function Ii(e,t){var n=e.ref;if(n!==null)if(typeof n=="function")try{n(null)}catch(o){Dt(e,t,o)}else n.current=null}function Lu(e,t,n){try{n()}catch(o){Dt(e,t,o)}}var pp=!1;function C5(e,t){if(uu=La,e=dm(),Ku(e)){if("selectionStart"in e)var n={start:e.selectionStart,end:e.selectionEnd};else e:{n=(n=e.ownerDocument)&&n.defaultView||window;var o=n.getSelection&&n.getSelection();if(o&&o.rangeCount!==0){n=o.anchorNode;var r=o.anchorOffset,i=o.focusNode;o=o.focusOffset;try{n.nodeType,i.nodeType}catch{n=null;break e}var l=0,s=-1,a=-1,c=0,f=0,u=e,x=null;t:for(;;){for(var S;u!==n||r!==0&&u.nodeType!==3||(s=l+r),u!==i||o!==0&&u.nodeType!==3||(a=l+o),u.nodeType===3&&(l+=u.nodeValue.length),(S=u.firstChild)!==null;)x=u,u=S;for(;;){if(u===e)break t;if(x===n&&++c===r&&(s=l),x===i&&++f===o&&(a=l),(S=u.nextSibling)!==null)break;u=x,x=u.parentNode}u=S}n=s===-1||a===-1?null:{start:s,end:a}}else n=null}n=n||{start:0,end:0}}else n=null;for(_u={focusedElem:e,selectionRange:n},La=!1,re=t;re!==null;)if(t=re,e=t.child,(t.subtreeFlags&1028)!==0&&e!==null)e.return=t,re=e;else for(;re!==null;){t=re;try{var b=t.alternate;if((t.flags&1024)!==0)switch(t.tag){case 0:case 11:case 15:break;case 1:if(b!==null){var N=b.memoizedProps,E=b.memoizedState,g=t.stateNode,v=g.getSnapshotBeforeUpdate(t.elementType===t.type?N:ao(t.type,N),E);g.__reactInternalSnapshotBeforeUpdate=v}break;case 3:var h=t.stateNode.containerInfo;h.nodeType===1?h.textContent="":h.nodeType===9&&h.documentElement&&h.removeChild(h.documentElement);break;case 5:case 6:case 4:case 17:break;default:throw Error(Q(163))}}catch(C){Dt(t,t.return,C)}if(e=t.sibling,e!==null){e.return=t.return,re=e;break}re=t.return}return b=pp,pp=!1,b}function Ol(e,t,n){var o=t.updateQueue;if(o=o!==null?o.lastEffect:null,o!==null){var r=o=o.next;do{if((r.tag&e)===e){var i=r.destroy;r.destroy=void 0,i!==void 0&&Lu(t,n,i)}r=r.next}while(r!==o)}}function tc(e,t){if(t=t.updateQueue,t=t!==null?t.lastEffect:null,t!==null){var n=t=t.next;do{if((n.tag&e)===e){var o=n.create;n.destroy=o()}n=n.next}while(n!==t)}}function Nu(e){var t=e.ref;if(t!==null){var n=e.stateNode;e.tag,e=n,typeof t=="function"?t(e):t.current=e}}function l0(e){var t=e.alternate;t!==null&&(e.alternate=null,l0(t)),e.child=null,e.deletions=null,e.sibling=null,e.tag===5&&(t=e.stateNode,t!==null&&(delete t[Mo],delete t[Kl],delete t[pu],delete t[l5],delete t[s5])),e.stateNode=null,e.return=null,e.dependencies=null,e.memoizedProps=null,e.memoizedState=null,e.pendingProps=null,e.stateNode=null,e.updateQueue=null}function s0(e){return e.tag===5||e.tag===3||e.tag===4}function mp(e){e:for(;;){for(;e.sibling===null;){if(e.return===null||s0(e.return))return null;e=e.return}for(e.sibling.return=e.return,e=e.sibling;e.tag!==5&&e.tag!==6&&e.tag!==18;){if(e.flags&2||e.child===null||e.tag===4)continue e;e.child.return=e,e=e.child}if(!(e.flags&2))return e.stateNode}}function Iu(e,t,n){var o=e.tag;if(o===5||o===6)e=e.stateNode,t?n.nodeType===8?n.parentNode.insertBefore(e,t):n.insertBefore(e,t):(n.nodeType===8?(t=n.parentNode,t.insertBefore(e,n)):(t=n,t.appendChild(e)),n=n._reactRootContainer,n!=null||t.onclick!==null||(t.onclick=Ra));else if(o!==4&&(e=e.child,e!==null))for(Iu(e,t,n),e=e.sibling;e!==null;)Iu(e,t,n),e=e.sibling}function Ru(e,t,n){var o=e.tag;if(o===5||o===6)e=e.stateNode,t?n.insertBefore(e,t):n.appendChild(e);else if(o!==4&&(e=e.child,e!==null))for(Ru(e,t,n),e=e.sibling;e!==null;)Ru(e,t,n),e=e.sibling}var Jt=null,co=!1;function nr(e,t,n){for(n=n.child;n!==null;)a0(e,t,n),n=n.sibling}function a0(e,t,n){if(Eo&&typeof Eo.onCommitFiberUnmount=="function")try{Eo.onCommitFiberUnmount(Va,n)}catch{}switch(n.tag){case 5:_n||Ii(n,t);case 6:var o=Jt,r=co;Jt=null,nr(e,t,n),Jt=o,co=r,Jt!==null&&(co?(e=Jt,n=n.stateNode,e.nodeType===8?e.parentNode.removeChild(n):e.removeChild(n)):Jt.removeChild(n.stateNode));break;case 18:Jt!==null&&(co?(e=Jt,n=n.stateNode,e.nodeType===8?Pd(e.parentNode,n):e.nodeType===1&&Pd(e,n),Ql(e)):Pd(Jt,n.stateNode));break;case 4:o=Jt,r=co,Jt=n.stateNode.containerInfo,co=!0,nr(e,t,n),Jt=o,co=r;break;case 0:case 11:case 14:case 15:if(!_n&&(o=n.updateQueue,o!==null&&(o=o.lastEffect,o!==null))){r=o=o.next;do{var i=r,l=i.destroy;i=i.tag,l!==void 0&&((i&2)!==0||(i&4)!==0)&&Lu(n,t,l),r=r.next}while(r!==o)}nr(e,t,n);break;case 1:if(!_n&&(Ii(n,t),o=n.stateNode,typeof o.componentWillUnmount=="function"))try{o.props=n.memoizedProps,o.state=n.memoizedState,o.componentWillUnmount()}catch(s){Dt(n,t,s)}nr(e,t,n);break;case 21:nr(e,t,n);break;case 22:n.mode&1?(_n=(o=_n)||n.memoizedState!==null,nr(e,t,n),_n=o):nr(e,t,n);break;default:nr(e,t,n)}}function gp(e){var t=e.updateQueue;if(t!==null){e.updateQueue=null;var n=e.stateNode;n===null&&(n=e.stateNode=new k5),t.forEach(function(o){var r=T5.bind(null,e,o);n.has(o)||(n.add(o),o.then(r,r))})}}function so(e,t){var n=t.deletions;if(n!==null)for(var o=0;o<n.length;o++){var r=n[o];try{var i=e,l=t,s=l;e:for(;s!==null;){switch(s.tag){case 5:Jt=s.stateNode,co=!1;break e;case 3:Jt=s.stateNode.containerInfo,co=!0;break e;case 4:Jt=s.stateNode.containerInfo,co=!0;break e}s=s.return}if(Jt===null)throw Error(Q(160));a0(i,l,r),Jt=null,co=!1;var a=r.alternate;a!==null&&(a.return=null),r.return=null}catch(c){Dt(r,t,c)}}if(t.subtreeFlags&12854)for(t=t.child;t!==null;)c0(t,e),t=t.sibling}function c0(e,t){var n=e.alternate,o=e.flags;switch(e.tag){case 0:case 11:case 14:case 15:if(so(t,e),Co(e),o&4){try{Ol(3,e,e.return),tc(3,e)}catch(N){Dt(e,e.return,N)}try{Ol(5,e,e.return)}catch(N){Dt(e,e.return,N)}}break;case 1:so(t,e),Co(e),o&512&&n!==null&&Ii(n,n.return);break;case 5:if(so(t,e),Co(e),o&512&&n!==null&&Ii(n,n.return),e.flags&32){var r=e.stateNode;try{jl(r,"")}catch(N){Dt(e,e.return,N)}}if(o&4&&(r=e.stateNode,r!=null)){var i=e.memoizedProps,l=n!==null?n.memoizedProps:i,s=e.type,a=e.updateQueue;if(e.updateQueue=null,a!==null)try{s==="input"&&i.type==="radio"&&i.name!=null&&Rp(r,i),tu(s,l);var c=tu(s,i);for(l=0;l<a.length;l+=2){var f=a[l],u=a[l+1];f==="style"?Bp(r,u):f==="dangerouslySetInnerHTML"?Pp(r,u):f==="children"?jl(r,u):Ou(r,f,u,c)}switch(s){case"input":qd(r,i);break;case"textarea":$p(r,i);break;case"select":var x=r._wrapperState.wasMultiple;r._wrapperState.wasMultiple=!!i.multiple;var S=i.value;S!=null?$i(r,!!i.multiple,S,!1):x!==!!i.multiple&&(i.defaultValue!=null?$i(r,!!i.multiple,i.defaultValue,!0):$i(r,!!i.multiple,i.multiple?[]:"",!1))}r[Kl]=i}catch(N){Dt(e,e.return,N)}}break;case 6:if(so(t,e),Co(e),o&4){if(e.stateNode===null)throw Error(Q(162));r=e.stateNode,i=e.memoizedProps;try{r.nodeValue=i}catch(N){Dt(e,e.return,N)}}break;case 3:if(so(t,e),Co(e),o&4&&n!==null&&n.memoizedState.isDehydrated)try{Ql(t.containerInfo)}catch(N){Dt(e,e.return,N)}break;case 4:so(t,e),Co(e);break;case 13:so(t,e),Co(e),r=e.child,r.flags&8192&&(i=r.memoizedState!==null,r.stateNode.isHidden=i,!i||r.alternate!==null&&r.alternate.memoizedState!==null||(g_=zt())),o&4&&gp(e);break;case 22:if(f=n!==null&&n.memoizedState!==null,e.mode&1?(_n=(c=_n)||f,so(t,e),_n=c):so(t,e),Co(e),o&8192){if(c=e.memoizedState!==null,(e.stateNode.isHidden=c)&&!f&&(e.mode&1)!==0)for(re=e,f=e.child;f!==null;){for(u=re=f;re!==null;){switch(x=re,S=x.child,x.tag){case 0:case 11:case 14:case 15:Ol(4,x,x.return);break;case 1:Ii(x,x.return);var b=x.stateNode;if(typeof b.componentWillUnmount=="function"){o=x,n=x.return;try{t=o,b.props=t.memoizedProps,b.state=t.memoizedState,b.componentWillUnmount()}catch(N){Dt(o,n,N)}}break;case 5:Ii(x,x.return);break;case 22:if(x.memoizedState!==null){xp(u);continue}}S!==null?(S.return=x,re=S):xp(u)}f=f.sibling}e:for(f=null,u=e;;){if(u.tag===5){if(f===null){f=u;try{r=u.stateNode,c?(i=r.style,typeof i.setProperty=="function"?i.setProperty("display","none","important"):i.display="none"):(s=u.stateNode,a=u.memoizedProps.style,l=a!=null&&a.hasOwnProperty("display")?a.display:null,s.style.display=Dp("display",l))}catch(N){Dt(e,e.return,N)}}}else if(u.tag===6){if(f===null)try{u.stateNode.nodeValue=c?"":u.memoizedProps}catch(N){Dt(e,e.return,N)}}else if((u.tag!==22&&u.tag!==23||u.memoizedState===null||u===e)&&u.child!==null){u.child.return=u,u=u.child;continue}if(u===e)break e;for(;u.sibling===null;){if(u.return===null||u.return===e)break e;f===u&&(f=null),u=u.return}f===u&&(f=null),u.sibling.return=u.return,u=u.sibling}}break;case 19:so(t,e),Co(e),o&4&&gp(e);break;case 21:break;default:so(t,e),Co(e)}}function Co(e){var t=e.flags;if(t&2){try{e:{for(var n=e.return;n!==null;){if(s0(n)){var o=n;break e}n=n.return}throw Error(Q(160))}switch(o.tag){case 5:var r=o.stateNode;o.flags&32&&(jl(r,""),o.flags&=-33);var i=mp(e);Ru(e,i,r);break;case 3:case 4:var l=o.stateNode.containerInfo,s=mp(e);Iu(e,s,l);break;default:throw Error(Q(161))}}catch(a){Dt(e,e.return,a)}e.flags&=-3}t&4096&&(e.flags&=-4097)}function S5(e,t,n){re=e,d0(e,t,n)}function d0(e,t,n){for(var o=(e.mode&1)!==0;re!==null;){var r=re,i=r.child;if(r.tag===22&&o){var l=r.memoizedState!==null||ua;if(!l){var s=r.alternate,a=s!==null&&s.memoizedState!==null||_n;s=ua;var c=_n;if(ua=l,(_n=a)&&!c)for(re=r;re!==null;)l=re,a=l.child,l.tag===22&&l.memoizedState!==null?vp(r):a!==null?(a.return=l,re=a):vp(r);for(;i!==null;)re=i,d0(i,t,n),i=i.sibling;re=r,ua=s,_n=c}yp(e,t,n)}else(r.subtreeFlags&8772)!==0&&i!==null?(i.return=r,re=i):yp(e,t,n)}}function yp(e){for(;re!==null;){var t=re;if((t.flags&8772)!==0){var n=t.alternate;try{if((t.flags&8772)!==0)switch(t.tag){case 0:case 11:case 15:_n||tc(5,t);break;case 1:var o=t.stateNode;if(t.flags&4&&!_n)if(n===null)o.componentDidMount();else{var r=t.elementType===t.type?n.memoizedProps:ao(t.type,n.memoizedProps);o.componentDidUpdate(r,n.memoizedState,o.__reactInternalSnapshotBeforeUpdate)}var i=t.updateQueue;i!==null&&np(t,i,o);break;case 3:var l=t.updateQueue;if(l!==null){if(n=null,t.child!==null)switch(t.child.tag){case 5:n=t.child.stateNode;break;case 1:n=t.child.stateNode}np(t,l,n)}break;case 5:var s=t.stateNode;if(n===null&&t.flags&4){n=s;var a=t.memoizedProps;switch(t.type){case"button":case"input":case"select":case"textarea":a.autoFocus&&n.focus();break;case"img":a.src&&(n.src=a.src)}}break;case 6:break;case 4:break;case 12:break;case 13:if(t.memoizedState===null){var c=t.alternate;if(c!==null){var f=c.memoizedState;if(f!==null){var u=f.dehydrated;u!==null&&Ql(u)}}}break;case 19:case 17:case 21:case 22:case 23:case 25:break;default:throw Error(Q(163))}_n||t.flags&512&&Nu(t)}catch(x){Dt(t,t.return,x)}}if(t===e){re=null;break}if(n=t.sibling,n!==null){n.return=t.return,re=n;break}re=t.return}}function xp(e){for(;re!==null;){var t=re;if(t===e){re=null;break}var n=t.sibling;if(n!==null){n.return=t.return,re=n;break}re=t.return}}function vp(e){for(;re!==null;){var t=re;try{switch(t.tag){case 0:case 11:case 15:var n=t.return;try{tc(4,t)}catch(a){Dt(t,n,a)}break;case 1:var o=t.stateNode;if(typeof o.componentDidMount=="function"){var r=t.return;try{o.componentDidMount()}catch(a){Dt(t,r,a)}}var i=t.return;try{Nu(t)}catch(a){Dt(t,i,a)}break;case 5:var l=t.return;try{Nu(t)}catch(a){Dt(t,l,a)}}}catch(a){Dt(t,t.return,a)}if(t===e){re=null;break}var s=t.sibling;if(s!==null){s.return=t.return,re=s;break}re=t.return}}var M5=Math.ceil,ja=Go.ReactCurrentDispatcher,p_=Go.ReactCurrentOwner,Gn=Go.ReactCurrentBatchConfig,Ge=0,Vt=null,Ft=null,Zt=0,Rn=0,Ri=vr(0),Ut=0,os=null,Zr=0,nc=0,m_=0,Al=null,Cn=null,g_=0,Ui=1/0,Fo=null,Ha=!1,$u=null,hr=null,_a=!1,ar=null,Ua=0,Fl=0,Tu=null,va=-1,wa=0;function yn(){return(Ge&6)!==0?zt():va!==-1?va:va=zt()}function pr(e){return(e.mode&1)===0?1:(Ge&2)!==0&&Zt!==0?Zt&-Zt:c5.transition!==null?(wa===0&&(wa=Xp()),wa):(e=dt,e!==0||(e=window.event,e=e===void 0?16:tm(e.type)),e)}function fo(e,t,n,o){if(50<Fl)throw Fl=0,Tu=null,Error(Q(185));rs(e,n,o),((Ge&2)===0||e!==Vt)&&(e===Vt&&((Ge&2)===0&&(nc|=n),Ut===4&&lr(e,Zt)),Ln(e,o),n===1&&Ge===0&&(t.mode&1)===0&&(Ui=zt()+500,Ja&&wr()))}function Ln(e,t){var n=e.callbackNode;uy(e,t);var o=Ea(e,e===Vt?Zt:0);if(o===0)n!==null&&Lh(n),e.callbackNode=null,e.callbackPriority=0;else if(t=o&-o,e.callbackPriority!==t){if(n!=null&&Lh(n),t===1)e.tag===0?a5(wp.bind(null,e)):vm(wp.bind(null,e)),r5(function(){(Ge&6)===0&&wr()}),n=null;else{switch(Gp(o)){case 1:n=Hu;break;case 4:n=Qp;break;case 16:n=Ma;break;case 536870912:n=Vp;break;default:n=Ma}n=y0(n,u0.bind(null,e))}e.callbackPriority=t,e.callbackNode=n}}function u0(e,t){if(va=-1,wa=0,(Ge&6)!==0)throw Error(Q(327));var n=e.callbackNode;if(zi()&&e.callbackNode!==n)return null;var o=Ea(e,e===Vt?Zt:0);if(o===0)return null;if((o&30)!==0||(o&e.expiredLanes)!==0||t)t=Ya(e,o);else{t=o;var r=Ge;Ge|=2;var i=f0();(Vt!==e||Zt!==t)&&(Fo=null,Ui=zt()+500,Xr(e,t));do try{N5();break}catch(s){_0(e,s)}while(!0);n_(),ja.current=i,Ge=r,Ft!==null?t=0:(Vt=null,Zt=0,t=Ut)}if(t!==0){if(t===2&&(r=lu(e),r!==0&&(o=r,t=Pu(e,r))),t===1)throw n=os,Xr(e,0),lr(e,o),Ln(e,zt()),n;if(t===6)lr(e,o);else{if(r=e.current.alternate,(o&30)===0&&!E5(r)&&(t=Ya(e,o),t===2&&(i=lu(e),i!==0&&(o=i,t=Pu(e,i))),t===1))throw n=os,Xr(e,0),lr(e,o),Ln(e,zt()),n;switch(e.finishedWork=r,e.finishedLanes=o,t){case 0:case 1:throw Error(Q(345));case 2:Ur(e,Cn,Fo);break;case 3:if(lr(e,o),(o&130023424)===o&&(t=g_+500-zt(),10<t)){if(Ea(e,0)!==0)break;if(r=e.suspendedLanes,(r&o)!==o){yn(),e.pingedLanes|=e.suspendedLanes&r;break}e.timeoutHandle=hu(Ur.bind(null,e,Cn,Fo),t);break}Ur(e,Cn,Fo);break;case 4:if(lr(e,o),(o&4194240)===o)break;for(t=e.eventTimes,r=-1;0<o;){var l=31-_o(o);i=1<<l,l=t[l],l>r&&(r=l),o&=~i}if(o=r,o=zt()-o,o=(120>o?120:480>o?480:1080>o?1080:1920>o?1920:3e3>o?3e3:4320>o?4320:1960*M5(o/1960))-o,10<o){e.timeoutHandle=hu(Ur.bind(null,e,Cn,Fo),o);break}Ur(e,Cn,Fo);break;case 5:Ur(e,Cn,Fo);break;default:throw Error(Q(329))}}}return Ln(e,zt()),e.callbackNode===n?u0.bind(null,e):null}function Pu(e,t){var n=Al;return e.current.memoizedState.isDehydrated&&(Xr(e,t).flags|=256),e=Ya(e,t),e!==2&&(t=Cn,Cn=n,t!==null&&Du(t)),e}function Du(e){Cn===null?Cn=e:Cn.push.apply(Cn,e)}function E5(e){for(var t=e;;){if(t.flags&16384){var n=t.updateQueue;if(n!==null&&(n=n.stores,n!==null))for(var o=0;o<n.length;o++){var r=n[o],i=r.getSnapshot;r=r.value;try{if(!ho(i(),r))return!1}catch{return!1}}}if(n=t.child,t.subtreeFlags&16384&&n!==null)n.return=t,t=n;else{if(t===e)break;for(;t.sibling===null;){if(t.return===null||t.return===e)return!0;t=t.return}t.sibling.return=t.return,t=t.sibling}}return!0}function lr(e,t){for(t&=~m_,t&=~nc,e.suspendedLanes|=t,e.pingedLanes&=~t,e=e.expirationTimes;0<t;){var n=31-_o(t),o=1<<n;e[n]=-1,t&=~o}}function wp(e){if((Ge&6)!==0)throw Error(Q(327));zi();var t=Ea(e,0);if((t&1)===0)return Ln(e,zt()),null;var n=Ya(e,t);if(e.tag!==0&&n===2){var o=lu(e);o!==0&&(t=o,n=Pu(e,o))}if(n===1)throw n=os,Xr(e,0),lr(e,t),Ln(e,zt()),n;if(n===6)throw Error(Q(345));return e.finishedWork=e.current.alternate,e.finishedLanes=t,Ur(e,Cn,Fo),Ln(e,zt()),null}function y_(e,t){var n=Ge;Ge|=1;try{return e(t)}finally{Ge=n,Ge===0&&(Ui=zt()+500,Ja&&wr())}}function ei(e){ar!==null&&ar.tag===0&&(Ge&6)===0&&zi();var t=Ge;Ge|=1;var n=Gn.transition,o=dt;try{if(Gn.transition=null,dt=1,e)return e()}finally{dt=o,Gn.transition=n,Ge=t,(Ge&6)===0&&wr()}}function x_(){Rn=Ri.current,Mt(Ri)}function Xr(e,t){e.finishedWork=null,e.finishedLanes=0;var n=e.timeoutHandle;if(n!==-1&&(e.timeoutHandle=-1,o5(n)),Ft!==null)for(n=Ft.return;n!==null;){var o=n;switch(Zu(o),o.tag){case 1:o=o.type.childContextTypes,o!=null&&$a();break;case 3:ji(),Mt(Mn),Mt(fn),a_();break;case 5:s_(o);break;case 4:ji();break;case 13:Mt(Nt);break;case 19:Mt(Nt);break;case 10:o_(o.type._context);break;case 22:case 23:x_()}n=n.return}if(Vt=e,Ft=e=mr(e.current,null),Zt=Rn=t,Ut=0,os=null,m_=nc=Zr=0,Cn=Al=null,Qr!==null){for(t=0;t<Qr.length;t++)if(n=Qr[t],o=n.interleaved,o!==null){n.interleaved=null;var r=o.next,i=n.pending;if(i!==null){var l=i.next;i.next=r,o.next=l}n.pending=o}Qr=null}return e}function _0(e,t){do{var n=Ft;try{if(n_(),ga.current=Wa,Fa){for(var o=It.memoizedState;o!==null;){var r=o.queue;r!==null&&(r.pending=null),o=o.next}Fa=!1}if(Jr=0,Qt=Ht=It=null,zl=!1,es=0,p_.current=null,n===null||n.return===null){Ut=1,os=t,Ft=null;break}e:{var i=e,l=n.return,s=n,a=t;if(t=Zt,s.flags|=32768,a!==null&&typeof a=="object"&&typeof a.then=="function"){var c=a,f=s,u=f.tag;if((f.mode&1)===0&&(u===0||u===11||u===15)){var x=f.alternate;x?(f.updateQueue=x.updateQueue,f.memoizedState=x.memoizedState,f.lanes=x.lanes):(f.updateQueue=null,f.memoizedState=null)}var S=ap(l);if(S!==null){S.flags&=-257,cp(S,l,s,i,t),S.mode&1&&sp(i,c,t),t=S,a=c;var b=t.updateQueue;if(b===null){var N=new Set;N.add(a),t.updateQueue=N}else b.add(a);break e}else{if((t&1)===0){sp(i,c,t),v_();break e}a=Error(Q(426))}}else if(Lt&&s.mode&1){var E=ap(l);if(E!==null){(E.flags&65536)===0&&(E.flags|=256),cp(E,l,s,i,t),e_(Hi(a,s));break e}}i=a=Hi(a,s),Ut!==4&&(Ut=2),Al===null?Al=[i]:Al.push(i),i=l;do{switch(i.tag){case 3:i.flags|=65536,t&=-t,i.lanes|=t;var g=Gm(i,a,t);tp(i,g);break e;case 1:s=a;var v=i.type,h=i.stateNode;if((i.flags&128)===0&&(typeof v.getDerivedStateFromError=="function"||h!==null&&typeof h.componentDidCatch=="function"&&(hr===null||!hr.has(h)))){i.flags|=65536,t&=-t,i.lanes|=t;var C=qm(i,s,t);tp(i,C);break e}}i=i.return}while(i!==null)}p0(n)}catch(U){t=U,Ft===n&&n!==null&&(Ft=n=n.return);continue}break}while(!0)}function f0(){var e=ja.current;return ja.current=Wa,e===null?Wa:e}function v_(){(Ut===0||Ut===3||Ut===2)&&(Ut=4),Vt===null||(Zr&268435455)===0&&(nc&268435455)===0||lr(Vt,Zt)}function Ya(e,t){var n=Ge;Ge|=2;var o=f0();(Vt!==e||Zt!==t)&&(Fo=null,Xr(e,t));do try{L5();break}catch(r){_0(e,r)}while(!0);if(n_(),Ge=n,ja.current=o,Ft!==null)throw Error(Q(261));return Vt=null,Zt=0,Ut}function L5(){for(;Ft!==null;)h0(Ft)}function N5(){for(;Ft!==null&&!ny();)h0(Ft)}function h0(e){var t=g0(e.alternate,e,Rn);e.memoizedProps=e.pendingProps,t===null?p0(e):Ft=t,p_.current=null}function p0(e){var t=e;do{var n=t.alternate;if(e=t.return,(t.flags&32768)===0){if(n=w5(n,t,Rn),n!==null){Ft=n;return}}else{if(n=b5(n,t),n!==null){n.flags&=32767,Ft=n;return}if(e!==null)e.flags|=32768,e.subtreeFlags=0,e.deletions=null;else{Ut=6,Ft=null;return}}if(t=t.sibling,t!==null){Ft=t;return}Ft=t=e}while(t!==null);Ut===0&&(Ut=5)}function Ur(e,t,n){var o=dt,r=Gn.transition;try{Gn.transition=null,dt=1,I5(e,t,n,o)}finally{Gn.transition=r,dt=o}return null}function I5(e,t,n,o){do zi();while(ar!==null);if((Ge&6)!==0)throw Error(Q(327));n=e.finishedWork;var r=e.finishedLanes;if(n===null)return null;if(e.finishedWork=null,e.finishedLanes=0,n===e.current)throw Error(Q(177));e.callbackNode=null,e.callbackPriority=0;var i=n.lanes|n.childLanes;if(_y(e,i),e===Vt&&(Ft=Vt=null,Zt=0),(n.subtreeFlags&2064)===0&&(n.flags&2064)===0||_a||(_a=!0,y0(Ma,function(){return zi(),null})),i=(n.flags&15990)!==0,(n.subtreeFlags&15990)!==0||i){i=Gn.transition,Gn.transition=null;var l=dt;dt=1;var s=Ge;Ge|=4,p_.current=null,C5(e,n),c0(n,e),Jy(_u),La=!!uu,_u=uu=null,e.current=n,S5(n,e,r),oy(),Ge=s,dt=l,Gn.transition=i}else e.current=n;if(_a&&(_a=!1,ar=e,Ua=r),i=e.pendingLanes,i===0&&(hr=null),ly(n.stateNode,o),Ln(e,zt()),t!==null)for(o=e.onRecoverableError,n=0;n<t.length;n++)r=t[n],o(r.value,{componentStack:r.stack,digest:r.digest});if(Ha)throw Ha=!1,e=$u,$u=null,e;return(Ua&1)!==0&&e.tag!==0&&zi(),i=e.pendingLanes,(i&1)!==0?e===Tu?Fl++:(Fl=0,Tu=e):Fl=0,wr(),null}function zi(){if(ar!==null){var e=Gp(Ua),t=Gn.transition,n=dt;try{if(Gn.transition=null,dt=16>e?16:e,ar===null)var o=!1;else{if(e=ar,ar=null,Ua=0,(Ge&6)!==0)throw Error(Q(331));var r=Ge;for(Ge|=4,re=e.current;re!==null;){var i=re,l=i.child;if((re.flags&16)!==0){var s=i.deletions;if(s!==null){for(var a=0;a<s.length;a++){var c=s[a];for(re=c;re!==null;){var f=re;switch(f.tag){case 0:case 11:case 15:Ol(8,f,i)}var u=f.child;if(u!==null)u.return=f,re=u;else for(;re!==null;){f=re;var x=f.sibling,S=f.return;if(l0(f),f===c){re=null;break}if(x!==null){x.return=S,re=x;break}re=S}}}var b=i.alternate;if(b!==null){var N=b.child;if(N!==null){b.child=null;do{var E=N.sibling;N.sibling=null,N=E}while(N!==null)}}re=i}}if((i.subtreeFlags&2064)!==0&&l!==null)l.return=i,re=l;else e:for(;re!==null;){if(i=re,(i.flags&2048)!==0)switch(i.tag){case 0:case 11:case 15:Ol(9,i,i.return)}var g=i.sibling;if(g!==null){g.return=i.return,re=g;break e}re=i.return}}var v=e.current;for(re=v;re!==null;){l=re;var h=l.child;if((l.subtreeFlags&2064)!==0&&h!==null)h.return=l,re=h;else e:for(l=v;re!==null;){if(s=re,(s.flags&2048)!==0)try{switch(s.tag){case 0:case 11:case 15:tc(9,s)}}catch(U){Dt(s,s.return,U)}if(s===l){re=null;break e}var C=s.sibling;if(C!==null){C.return=s.return,re=C;break e}re=s.return}}if(Ge=r,wr(),Eo&&typeof Eo.onPostCommitFiberRoot=="function")try{Eo.onPostCommitFiberRoot(Va,e)}catch{}o=!0}return o}finally{dt=n,Gn.transition=t}}return!1}function bp(e,t,n){t=Hi(n,t),t=Gm(e,t,1),e=fr(e,t,1),t=yn(),e!==null&&(rs(e,1,t),Ln(e,t))}function Dt(e,t,n){if(e.tag===3)bp(e,e,n);else for(;t!==null;){if(t.tag===3){bp(t,e,n);break}else if(t.tag===1){var o=t.stateNode;if(typeof t.type.getDerivedStateFromError=="function"||typeof o.componentDidCatch=="function"&&(hr===null||!hr.has(o))){e=Hi(n,e),e=qm(t,e,1),t=fr(t,e,1),e=yn(),t!==null&&(rs(t,1,e),Ln(t,e));break}}t=t.return}}function R5(e,t,n){var o=e.pingCache;o!==null&&o.delete(t),t=yn(),e.pingedLanes|=e.suspendedLanes&n,Vt===e&&(Zt&n)===n&&(Ut===4||Ut===3&&(Zt&130023424)===Zt&&500>zt()-g_?Xr(e,0):m_|=n),Ln(e,t)}function m0(e,t){t===0&&((e.mode&1)===0?t=1:(t=Js,Js<<=1,(Js&130023424)===0&&(Js=4194304)));var n=yn();e=Vo(e,t),e!==null&&(rs(e,t,n),Ln(e,n))}function $5(e){var t=e.memoizedState,n=0;t!==null&&(n=t.retryLane),m0(e,n)}function T5(e,t){var n=0;switch(e.tag){case 13:var o=e.stateNode,r=e.memoizedState;r!==null&&(n=r.retryLane);break;case 19:o=e.stateNode;break;default:throw Error(Q(314))}o!==null&&o.delete(t),m0(e,n)}var g0;g0=function(e,t,n){if(e!==null)if(e.memoizedProps!==t.pendingProps||Mn.current)Sn=!0;else{if((e.lanes&n)===0&&(t.flags&128)===0)return Sn=!1,v5(e,t,n);Sn=(e.flags&131072)!==0}else Sn=!1,Lt&&(t.flags&1048576)!==0&&wm(t,Da,t.index);switch(t.lanes=0,t.tag){case 2:var o=t.type;xa(e,t),e=t.pendingProps;var r=Ai(t,fn.current);Bi(t,n),r=d_(null,t,o,e,r,n);var i=u_();return t.flags|=1,typeof r=="object"&&r!==null&&typeof r.render=="function"&&r.$$typeof===void 0?(t.tag=1,t.memoizedState=null,t.updateQueue=null,En(o)?(i=!0,Ta(t)):i=!1,t.memoizedState=r.state!==null&&r.state!==void 0?r.state:null,i_(t),r.updater=ec,t.stateNode=r,r._reactInternals=t,wu(t,o,e,n),t=Cu(null,t,o,!0,i,n)):(t.tag=0,Lt&&i&&Ju(t),gn(null,t,r,n),t=t.child),t;case 16:o=t.elementType;e:{switch(xa(e,t),e=t.pendingProps,r=o._init,o=r(o._payload),t.type=o,r=t.tag=D5(o),e=ao(o,e),r){case 0:t=ku(null,t,o,e,n);break e;case 1:t=_p(null,t,o,e,n);break e;case 11:t=dp(null,t,o,e,n);break e;case 14:t=up(null,t,o,ao(o.type,e),n);break e}throw Error(Q(306,o,""))}return t;case 0:return o=t.type,r=t.pendingProps,r=t.elementType===o?r:ao(o,r),ku(e,t,o,r,n);case 1:return o=t.type,r=t.pendingProps,r=t.elementType===o?r:ao(o,r),_p(e,t,o,r,n);case 3:e:{if(e0(t),e===null)throw Error(Q(387));o=t.pendingProps,i=t.memoizedState,r=i.element,Em(e,t),Oa(t,o,null,n);var l=t.memoizedState;if(o=l.element,i.isDehydrated)if(i={element:o,isDehydrated:!1,cache:l.cache,pendingSuspenseBoundaries:l.pendingSuspenseBoundaries,transitions:l.transitions},t.updateQueue.baseState=i,t.memoizedState=i,t.flags&256){r=Hi(Error(Q(423)),t),t=fp(e,t,o,n,r);break e}else if(o!==r){r=Hi(Error(Q(424)),t),t=fp(e,t,o,n,r);break e}else for($n=_r(t.stateNode.containerInfo.firstChild),Tn=t,Lt=!0,uo=null,n=Sm(t,null,o,n),t.child=n;n;)n.flags=n.flags&-3|4096,n=n.sibling;else{if(Fi(),o===r){t=Xo(e,t,n);break e}gn(e,t,o,n)}t=t.child}return t;case 5:return Lm(t),e===null&&yu(t),o=t.type,r=t.pendingProps,i=e!==null?e.memoizedProps:null,l=r.children,fu(o,r)?l=null:i!==null&&fu(o,i)&&(t.flags|=32),Zm(e,t),gn(e,t,l,n),t.child;case 6:return e===null&&yu(t),null;case 13:return t0(e,t,n);case 4:return l_(t,t.stateNode.containerInfo),o=t.pendingProps,e===null?t.child=Wi(t,null,o,n):gn(e,t,o,n),t.child;case 11:return o=t.type,r=t.pendingProps,r=t.elementType===o?r:ao(o,r),dp(e,t,o,r,n);case 7:return gn(e,t,t.pendingProps,n),t.child;case 8:return gn(e,t,t.pendingProps.children,n),t.child;case 12:return gn(e,t,t.pendingProps.children,n),t.child;case 10:e:{if(o=t.type._context,r=t.pendingProps,i=t.memoizedProps,l=r.value,wt(Ba,o._currentValue),o._currentValue=l,i!==null)if(ho(i.value,l)){if(i.children===r.children&&!Mn.current){t=Xo(e,t,n);break e}}else for(i=t.child,i!==null&&(i.return=t);i!==null;){var s=i.dependencies;if(s!==null){l=i.child;for(var a=s.firstContext;a!==null;){if(a.context===o){if(i.tag===1){a=Uo(-1,n&-n),a.tag=2;var c=i.updateQueue;if(c!==null){c=c.shared;var f=c.pending;f===null?a.next=a:(a.next=f.next,f.next=a),c.pending=a}}i.lanes|=n,a=i.alternate,a!==null&&(a.lanes|=n),xu(i.return,n,t),s.lanes|=n;break}a=a.next}}else if(i.tag===10)l=i.type===t.type?null:i.child;else if(i.tag===18){if(l=i.return,l===null)throw Error(Q(341));l.lanes|=n,s=l.alternate,s!==null&&(s.lanes|=n),xu(l,n,t),l=i.sibling}else l=i.child;if(l!==null)l.return=i;else for(l=i;l!==null;){if(l===t){l=null;break}if(i=l.sibling,i!==null){i.return=l.return,l=i;break}l=l.return}i=l}gn(e,t,r.children,n),t=t.child}return t;case 9:return r=t.type,o=t.pendingProps.children,Bi(t,n),r=qn(r),o=o(r),t.flags|=1,gn(e,t,o,n),t.child;case 14:return o=t.type,r=ao(o,t.pendingProps),r=ao(o.type,r),up(e,t,o,r,n);case 15:return Km(e,t,t.type,t.pendingProps,n);case 17:return o=t.type,r=t.pendingProps,r=t.elementType===o?r:ao(o,r),xa(e,t),t.tag=1,En(o)?(e=!0,Ta(t)):e=!1,Bi(t,n),Xm(t,o,r),wu(t,o,r,n),Cu(null,t,o,!0,e,n);case 19:return n0(e,t,n);case 22:return Jm(e,t,n)}throw Error(Q(156,t.tag))};function y0(e,t){return Yp(e,t)}function P5(e,t,n,o){this.tag=e,this.key=n,this.sibling=this.child=this.return=this.stateNode=this.type=this.elementType=null,this.index=0,this.ref=null,this.pendingProps=t,this.dependencies=this.memoizedState=this.updateQueue=this.memoizedProps=null,this.mode=o,this.subtreeFlags=this.flags=0,this.deletions=null,this.childLanes=this.lanes=0,this.alternate=null}function Xn(e,t,n,o){return new P5(e,t,n,o)}function w_(e){return e=e.prototype,!(!e||!e.isReactComponent)}function D5(e){if(typeof e=="function")return w_(e)?1:0;if(e!=null){if(e=e.$$typeof,e===Fu)return 11;if(e===Wu)return 14}return 2}function mr(e,t){var n=e.alternate;return n===null?(n=Xn(e.tag,t,e.key,e.mode),n.elementType=e.elementType,n.type=e.type,n.stateNode=e.stateNode,n.alternate=e,e.alternate=n):(n.pendingProps=t,n.type=e.type,n.flags=0,n.subtreeFlags=0,n.deletions=null),n.flags=e.flags&14680064,n.childLanes=e.childLanes,n.lanes=e.lanes,n.child=e.child,n.memoizedProps=e.memoizedProps,n.memoizedState=e.memoizedState,n.updateQueue=e.updateQueue,t=e.dependencies,n.dependencies=t===null?null:{lanes:t.lanes,firstContext:t.firstContext},n.sibling=e.sibling,n.index=e.index,n.ref=e.ref,n}function ba(e,t,n,o,r,i){var l=2;if(o=e,typeof e=="function")w_(e)&&(l=1);else if(typeof e=="string")l=5;else e:switch(e){case wi:return Gr(n.children,r,i,t);case Au:l=8,r|=8;break;case Yd:return e=Xn(12,n,t,r|2),e.elementType=Yd,e.lanes=i,e;case Qd:return e=Xn(13,n,t,r),e.elementType=Qd,e.lanes=i,e;case Vd:return e=Xn(19,n,t,r),e.elementType=Vd,e.lanes=i,e;case Lp:return oc(n,r,i,t);default:if(typeof e=="object"&&e!==null)switch(e.$$typeof){case Mp:l=10;break e;case Ep:l=9;break e;case Fu:l=11;break e;case Wu:l=14;break e;case or:l=16,o=null;break e}throw Error(Q(130,e==null?e:typeof e,""))}return t=Xn(l,n,t,r),t.elementType=e,t.type=o,t.lanes=i,t}function Gr(e,t,n,o){return e=Xn(7,e,o,t),e.lanes=n,e}function oc(e,t,n,o){return e=Xn(22,e,o,t),e.elementType=Lp,e.lanes=n,e.stateNode={isHidden:!1},e}function jd(e,t,n){return e=Xn(6,e,null,t),e.lanes=n,e}function Hd(e,t,n){return t=Xn(4,e.children!==null?e.children:[],e.key,t),t.lanes=n,t.stateNode={containerInfo:e.containerInfo,pendingChildren:null,implementation:e.implementation},t}function B5(e,t,n,o,r){this.tag=t,this.containerInfo=e,this.finishedWork=this.pingCache=this.current=this.pendingChildren=null,this.timeoutHandle=-1,this.callbackNode=this.pendingContext=this.context=null,this.callbackPriority=0,this.eventTimes=Md(0),this.expirationTimes=Md(-1),this.entangledLanes=this.finishedLanes=this.mutableReadLanes=this.expiredLanes=this.pingedLanes=this.suspendedLanes=this.pendingLanes=0,this.entanglements=Md(0),this.identifierPrefix=o,this.onRecoverableError=r,this.mutableSourceEagerHydrationData=null}function b_(e,t,n,o,r,i,l,s,a){return e=new B5(e,t,n,s,a),t===1?(t=1,i===!0&&(t|=8)):t=0,i=Xn(3,null,null,t),e.current=i,i.stateNode=e,i.memoizedState={element:o,isDehydrated:n,cache:null,transitions:null,pendingSuspenseBoundaries:null},i_(i),e}function z5(e,t,n){var o=3<arguments.length&&arguments[3]!==void 0?arguments[3]:null;return{$$typeof:vi,key:o==null?null:""+o,children:e,containerInfo:t,implementation:n}}function x0(e){if(!e)return yr;e=e._reactInternals;e:{if(ni(e)!==e||e.tag!==1)throw Error(Q(170));var t=e;do{switch(t.tag){case 3:t=t.stateNode.context;break e;case 1:if(En(t.type)){t=t.stateNode.__reactInternalMemoizedMergedChildContext;break e}}t=t.return}while(t!==null);throw Error(Q(171))}if(e.tag===1){var n=e.type;if(En(n))return xm(e,n,t)}return t}function v0(e,t,n,o,r,i,l,s,a){return e=b_(n,o,!0,e,r,i,l,s,a),e.context=x0(null),n=e.current,o=yn(),r=pr(n),i=Uo(o,r),i.callback=t??null,fr(n,i,r),e.current.lanes=r,rs(e,r,o),Ln(e,o),e}function rc(e,t,n,o){var r=t.current,i=yn(),l=pr(r);return n=x0(n),t.context===null?t.context=n:t.pendingContext=n,t=Uo(i,l),t.payload={element:e},o=o===void 0?null:o,o!==null&&(t.callback=o),e=fr(r,t,l),e!==null&&(fo(e,r,l,i),ma(e,r,l)),l}function Qa(e){return e=e.current,e.child?(e.child.tag===5,e.child.stateNode):null}function kp(e,t){if(e=e.memoizedState,e!==null&&e.dehydrated!==null){var n=e.retryLane;e.retryLane=n!==0&&n<t?n:t}}function k_(e,t){kp(e,t),(e=e.alternate)&&kp(e,t)}function O5(){return null}var w0=typeof reportError=="function"?reportError:function(e){console.error(e)};function C_(e){this._internalRoot=e}ic.prototype.render=C_.prototype.render=function(e){var t=this._internalRoot;if(t===null)throw Error(Q(409));rc(e,t,null,null)};ic.prototype.unmount=C_.prototype.unmount=function(){var e=this._internalRoot;if(e!==null){this._internalRoot=null;var t=e.containerInfo;ei(function(){rc(null,e,null,null)}),t[Qo]=null}};function ic(e){this._internalRoot=e}ic.prototype.unstable_scheduleHydration=function(e){if(e){var t=Jp();e={blockedOn:null,target:e,priority:t};for(var n=0;n<ir.length&&t!==0&&t<ir[n].priority;n++);ir.splice(n,0,e),n===0&&em(e)}};function S_(e){return!(!e||e.nodeType!==1&&e.nodeType!==9&&e.nodeType!==11)}function lc(e){return!(!e||e.nodeType!==1&&e.nodeType!==9&&e.nodeType!==11&&(e.nodeType!==8||e.nodeValue!==" react-mount-point-unstable "))}function Cp(){}function A5(e,t,n,o,r){if(r){if(typeof o=="function"){var i=o;o=function(){var c=Qa(l);i.call(c)}}var l=v0(t,o,e,0,null,!1,!1,"",Cp);return e._reactRootContainer=l,e[Qo]=l.current,Gl(e.nodeType===8?e.parentNode:e),ei(),l}for(;r=e.lastChild;)e.removeChild(r);if(typeof o=="function"){var s=o;o=function(){var c=Qa(a);s.call(c)}}var a=b_(e,0,!1,null,null,!1,!1,"",Cp);return e._reactRootContainer=a,e[Qo]=a.current,Gl(e.nodeType===8?e.parentNode:e),ei(function(){rc(t,a,n,o)}),a}function sc(e,t,n,o,r){var i=n._reactRootContainer;if(i){var l=i;if(typeof r=="function"){var s=r;r=function(){var a=Qa(l);s.call(a)}}rc(t,l,e,r)}else l=A5(n,t,e,r,o);return Qa(l)}qp=function(e){switch(e.tag){case 3:var t=e.stateNode;if(t.current.memoizedState.isDehydrated){var n=Il(t.pendingLanes);n!==0&&(Uu(t,n|1),Ln(t,zt()),(Ge&6)===0&&(Ui=zt()+500,wr()))}break;case 13:ei(function(){var o=Vo(e,1);if(o!==null){var r=yn();fo(o,e,1,r)}}),k_(e,1)}};Yu=function(e){if(e.tag===13){var t=Vo(e,134217728);if(t!==null){var n=yn();fo(t,e,134217728,n)}k_(e,134217728)}};Kp=function(e){if(e.tag===13){var t=pr(e),n=Vo(e,t);if(n!==null){var o=yn();fo(n,e,t,o)}k_(e,t)}};Jp=function(){return dt};Zp=function(e,t){var n=dt;try{return dt=e,t()}finally{dt=n}};ou=function(e,t,n){switch(t){case"input":if(qd(e,n),t=n.name,n.type==="radio"&&t!=null){for(n=e;n.parentNode;)n=n.parentNode;for(n=n.querySelectorAll("input[name="+JSON.stringify(""+t)+'][type="radio"]'),t=0;t<n.length;t++){var o=n[t];if(o!==e&&o.form===e.form){var r=Ka(o);if(!r)throw Error(Q(90));Ip(o),qd(o,r)}}}break;case"textarea":$p(e,n);break;case"select":t=n.value,t!=null&&$i(e,!!n.multiple,t,!1)}};Ap=y_;Fp=ei;var F5={usingClientEntryPoint:!1,Events:[ls,Si,Ka,zp,Op,y_]},Ml={findFiberByHostInstance:Yr,bundleType:0,version:"18.3.1",rendererPackageName:"react-dom"},W5={bundleType:Ml.bundleType,version:Ml.version,rendererPackageName:Ml.rendererPackageName,rendererConfig:Ml.rendererConfig,overrideHookState:null,overrideHookStateDeletePath:null,overrideHookStateRenamePath:null,overrideProps:null,overridePropsDeletePath:null,overridePropsRenamePath:null,setErrorHandler:null,setSuspenseHandler:null,scheduleUpdate:null,currentDispatcherRef:Go.ReactCurrentDispatcher,findHostInstanceByFiber:function(e){return e=Hp(e),e===null?null:e.stateNode},findFiberByHostInstance:Ml.findFiberByHostInstance||O5,findHostInstancesForRefresh:null,scheduleRefresh:null,scheduleRoot:null,setRefreshHandler:null,getCurrentFiber:null,reconcilerVersion:"18.3.1-next-f1338f8080-20240426"};if(typeof __REACT_DEVTOOLS_GLOBAL_HOOK__<"u"&&(El=__REACT_DEVTOOLS_GLOBAL_HOOK__,!El.isDisabled&&El.supportsFiber))try{Va=El.inject(W5),Eo=El}catch{}var El;Bn.__SECRET_INTERNALS_DO_NOT_USE_OR_YOU_WILL_BE_FIRED=F5;Bn.createPortal=function(e,t){var n=2<arguments.length&&arguments[2]!==void 0?arguments[2]:null;if(!S_(t))throw Error(Q(200));return z5(e,t,null,n)};Bn.createRoot=function(e,t){if(!S_(e))throw Error(Q(299));var n=!1,o="",r=w0;return t!=null&&(t.unstable_strictMode===!0&&(n=!0),t.identifierPrefix!==void 0&&(o=t.identifierPrefix),t.onRecoverableError!==void 0&&(r=t.onRecoverableError)),t=b_(e,1,!1,null,null,n,!1,o,r),e[Qo]=t.current,Gl(e.nodeType===8?e.parentNode:e),new C_(t)};Bn.findDOMNode=function(e){if(e==null)return null;if(e.nodeType===1)return e;var t=e._reactInternals;if(t===void 0)throw typeof e.render=="function"?Error(Q(188)):(e=Object.keys(e).join(","),Error(Q(268,e)));return e=Hp(t),e=e===null?null:e.stateNode,e};Bn.flushSync=function(e){return ei(e)};Bn.hydrate=function(e,t,n){if(!lc(t))throw Error(Q(200));return sc(null,e,t,!0,n)};Bn.hydrateRoot=function(e,t,n){if(!S_(e))throw Error(Q(405));var o=n!=null&&n.hydratedSources||null,r=!1,i="",l=w0;if(n!=null&&(n.unstable_strictMode===!0&&(r=!0),n.identifierPrefix!==void 0&&(i=n.identifierPrefix),n.onRecoverableError!==void 0&&(l=n.onRecoverableError)),t=v0(t,null,e,1,n??null,r,!1,i,l),e[Qo]=t.current,Gl(e),o)for(e=0;e<o.length;e++)n=o[e],r=n._getVersion,r=r(n._source),t.mutableSourceEagerHydrationData==null?t.mutableSourceEagerHydrationData=[n,r]:t.mutableSourceEagerHydrationData.push(n,r);return new ic(t)};Bn.render=function(e,t,n){if(!lc(t))throw Error(Q(200));return sc(null,e,t,!1,n)};Bn.unmountComponentAtNode=function(e){if(!lc(e))throw Error(Q(40));return e._reactRootContainer?(ei(function(){sc(null,null,e,!1,function(){e._reactRootContainer=null,e[Qo]=null})}),!0):!1};Bn.unstable_batchedUpdates=y_;Bn.unstable_renderSubtreeIntoContainer=function(e,t,n,o){if(!lc(n))throw Error(Q(200));if(e==null||e._reactInternals===void 0)throw Error(Q(38));return sc(e,t,n,!1,o)};Bn.version="18.3.1-next-f1338f8080-20240426"});var as=Wr((Ww,C0)=>{"use strict";function k0(){if(!(typeof __REACT_DEVTOOLS_GLOBAL_HOOK__>"u"||typeof __REACT_DEVTOOLS_GLOBAL_HOOK__.checkDCE!="function"))try{__REACT_DEVTOOLS_GLOBAL_HOOK__.checkDCE(k0)}catch(e){console.error(e)}}k0(),C0.exports=b0()});var M0=Wr(M_=>{"use strict";var S0=as();M_.createRoot=S0.createRoot,M_.hydrateRoot=S0.hydrateRoot;var jw});var Qg=xt(Et()),Vg=xt(M0());var Nc=xt(Et(),1),k=xt(Et(),1),Rc=xt(Et(),1),kg=xt(as(),1),Ji=xt(Et(),1),Zi=xt(Et(),1),Cg=xt(as(),1),Xt=xt(Et(),1),ys=xt(Et(),1),Mg=xt(Et(),1),hn=xt(Et(),1),Io=xt(Et(),1),Ng=xt(as(),1),ot=xt(Et(),1),Gt=xt(Et(),1),Mr=xt(Et(),1),Se=xt(Et(),1),Uv=xt(Et(),1),Nn=xt(Et(),1),Jn=xt(Et(),1),Sr=xt(Et(),1),Hg=xt(Et(),1),Tc=xt(Et(),1),Ug=xt(Et(),1);import{jsx as _2}from"react/jsx-runtime";import{jsx as $t,jsxs as br}from"react/jsx-runtime";import{jsx as T0}from"react/jsx-runtime";import{jsx as ve,jsxs as on}from"react/jsx-runtime";import{Fragment as L2,jsx as P0,jsxs as N2}from"react/jsx-runtime";import{jsx as B0}from"react/jsx-runtime";import{jsx as hc,jsxs as T2}from"react/jsx-runtime";import{jsx as y,jsxs as W}from"react/jsx-runtime";import{Fragment as qx,jsx as Ot,jsxs as z0}from"react/jsx-runtime";import{Fragment as F0,jsx as _,jsxs as J}from"react/jsx-runtime";import{Fragment as j0,jsx as Yt,jsxs as No}from"react/jsx-runtime";import{jsx as sg}from"react/jsx-runtime";import{jsx as ri,jsxs as cw}from"react/jsx-runtime";import{jsx as cg,jsxs as uw}from"react/jsx-runtime";import{jsx as B_,jsxs as fw}from"react/jsx-runtime";import{jsx as z_,jsxs as mw}from"react/jsx-runtime";import{jsx as De,jsxs as bt}from"react/jsx-runtime";import{jsx as _g,jsxs as xw}from"react/jsx-runtime";import{Fragment as bc,jsx as ce,jsxs as ht}from"react/jsx-runtime";var pg=["data-feedback-toolbar","data-annotation-popup","data-annotation-marker"],E_=pg.flatMap(e=>[`:not([${e}])`,`:not([${e}] *)`]).join(""),F_="feedback-freeze-styles",W_="__agentation_freeze";function j5(){return typeof window>"u"?{frozen:!1,installed:!0,origSetTimeout:setTimeout,origSetInterval:setInterval,origRAF:t=>0,pausedAnimations:[],frozenTimeoutQueue:[],frozenRAFQueue:[]}:window[W_]??{frozen:!1,installed:!1,origSetTimeout:window.setTimeout.bind(window),origSetInterval:window.setInterval.bind(window),origRAF:window.requestAnimationFrame.bind(window),pausedAnimations:[],frozenTimeoutQueue:[],frozenRAFQueue:[]}}var Ue=j5();function mg(){if(typeof window>"u")return;let e=window;Ue=e[W_]??(e[W_]=Ue),!Ue.installed&&(window.setTimeout=(t,n,...o)=>typeof t=="string"?Ue.origSetTimeout(t,n):Ue.origSetTimeout((...r)=>{Ue.frozen?Ue.frozenTimeoutQueue.push(()=>t(...r)):t(...r)},n,...o),window.setInterval=(t,n,...o)=>typeof t=="string"?Ue.origSetInterval(t,n):Ue.origSetInterval((...r)=>{Ue.frozen||t(...r)},n,...o),window.requestAnimationFrame=t=>Ue.origRAF(n=>{Ue.frozen?Ue.frozenRAFQueue.push(t):t(n)}),Ue.installed=!0)}var nt=Ue.origSetTimeout,gg=Ue.origSetInterval,Ec=Ue.origRAF;function H5(e){return e?pg.some(t=>!!e.closest?.(`[${t}]`)):!1}function U5(){if(typeof document>"u"||(mg(),Ue.frozen))return;Ue.frozen=!0,Ue.frozenTimeoutQueue=[],Ue.frozenRAFQueue=[];let e=document.getElementById(F_);e||(e=document.createElement("style"),e.id=F_),e.textContent=`
    *${E_},
    *${E_}::before,
    *${E_}::after {
      animation-play-state: paused !important;
      transition: none !important;
    }
  `,document.head.appendChild(e),Ue.pausedAnimations=[];try{document.getAnimations().forEach(t=>{if(t.playState!=="running")return;let n=t.effect?.target;H5(n)||(t.pause(),Ue.pausedAnimations.push(t))})}catch{}document.querySelectorAll("video").forEach(t=>{t.paused||(t.dataset.wasPaused="false",t.pause())})}function E0(){if(typeof document>"u"||!Ue.frozen)return;Ue.frozen=!1;let e=Ue.frozenTimeoutQueue;Ue.frozenTimeoutQueue=[];for(let n of e)Ue.origSetTimeout(()=>{if(Ue.frozen){Ue.frozenTimeoutQueue.push(n);return}try{n()}catch(o){console.warn("[agentation] Error replaying queued timeout:",o)}},0);let t=Ue.frozenRAFQueue;Ue.frozenRAFQueue=[];for(let n of t)Ue.origRAF(o=>{if(Ue.frozen){Ue.frozenRAFQueue.push(n);return}n(o)});for(let n of Ue.pausedAnimations)try{n.play()}catch(o){console.warn("[agentation] Error resuming animation:",o)}Ue.pausedAnimations=[],document.getElementById(F_)?.remove(),document.querySelectorAll("video").forEach(n=>{n.dataset.wasPaused==="false"&&(n.play().catch(()=>{}),delete n.dataset.wasPaused)})}function L0(){let e=(0,Nc.useMemo)(()=>{let t=0,n=new Set,o=()=>(n.forEach(clearTimeout),n.clear(),++t),r=l=>l===t;return{start:o,isCurrent:r,schedule:(l,s,a)=>{if(!r(l))return;let c=nt(()=>{n.delete(c),r(l)&&s()},a);n.add(c)}}},[]);return(0,Nc.useEffect)(()=>()=>{e.start()},[e]),e}function Y5(e,t,n,o){let r=c=>o.get(c.id)??c.id,i=new Map(e.map(c=>[r(c),c])),l=new Map(t.map(c=>[r(c),c])),s=new Set(n.map(c=>c.id)),a=[];for(let c of n){let f=i.get(c.id),u=l.get(c.id);if(f&&!u)continue;let x=u&&f&&u.comment!==f.comment;a.push(x?{...c,comment:u.comment}:c)}for(let[c,f]of l)!i.has(c)&&!s.has(c)&&a.push(f);return a}function vs(e){if(e.tagName!=="IFRAME")return null;try{return e.contentDocument}catch{return null}}function zn(e,t=document){if(e===t)return null;try{return e.defaultView?.frameElement}catch{return null}}function X_(e){return e.nodeType===11&&"host"in e}function Ki(e){let t=e.getBoundingClientRect(),n=e.offsetWidth?t.width/e.offsetWidth:1,o=e.offsetHeight?t.height/e.offsetHeight:1,r=e.ownerDocument.defaultView?.getComputedStyle(e),i=f=>parseFloat(f||"0")||0,l=i(r?.paddingLeft),s=i(r?.paddingTop),a=e.clientWidth-l-i(r?.paddingRight),c=e.clientHeight-s-i(r?.paddingBottom);return{x:t.left+(e.clientLeft+l)*n,y:t.top+(e.clientTop+s)*o,sx:n,sy:o,width:a*n,height:c*o}}function N0(e){try{let t=new URL(e);return t.origin+t.pathname}catch{return e}}function Q5(e,t,n,o=document){let r=zn(e,o);for(;r;){let i=Ki(r);t=i.x+t*i.sx,n=i.y+n*i.sy,r=zn(r.ownerDocument,o)}return{x:t,y:n}}function Wt(e,t=document){let n=e.getBoundingClientRect();return yg(e.ownerDocument,n,t)}function yg(e,t,n){if(!zn(e,n))return t;let o=t.left,r=t.top,i=t.right,l=t.bottom,s=zn(e,n);for(;s;){let a=Ki(s);o=Math.max(a.x,a.x+o*a.sx),r=Math.max(a.y,a.y+r*a.sy),i=Math.min(a.x+a.width,a.x+i*a.sx),l=Math.min(a.y+a.height,a.y+l*a.sy),s=zn(s.ownerDocument,n)}return new DOMRect(o,r,Math.max(0,i-o),Math.max(0,l-r))}function Ic(e){let t=[];for(let n of e.querySelectorAll("*"))n.tagName==="IFRAME"&&t.push(n),n.shadowRoot&&n.tagName!=="AGENTATION-TOOLBAR"&&t.push(...Ic(n.shadowRoot));return t}function V5(e,t,n,o=document){let r=[];for(let u=zn(e.ownerDocument,o);u;u=zn(u.ownerDocument,o))r.unshift(u);if(!r.length)return;let i=r.map(u=>{let x=Ki(u);return t=(t-x.x)/x.sx,n=(n-x.y)/x.sy,{index:Ic(u.ownerDocument).indexOf(u),id:u.id||void 0,url:vs(u)?.URL??""}}),l=e.ownerDocument.defaultView,s=!1;for(let u=e;u;u=u.parentElement)if(["fixed","sticky"].includes(l.getComputedStyle(u).position)){s=!0;break}let a=s?0:l.scrollX,c=s?0:l.scrollY,f=e.getBoundingClientRect();return{path:i,x:t+a,y:n+c,fixed:s,boundingBox:{x:f.left+a,y:f.top+c,width:f.width,height:f.height}}}function X5(e=document){let t=new Map,n=o=>{let r=t.get(o);return r||(r=Ic(o),t.set(o,r)),r};return o=>G5(o,e,n)}function G5(e,t=document,n=Ic){let o=e.frame;if(!o)return e;let r=t;for(let S of o.path){let b=n(r),N=S.id?b.find(g=>g.id===S.id):b[S.index],E=N&&vs(N);if(!E||N0(E.URL)!==N0(S.url))return null;r=E}let i=r.defaultView,l=o.fixed?0:i.scrollX,s=o.fixed?0:i.scrollY,a=o.x-l,c=o.y-s;for(let S=r,b=zn(r,t);b;b=zn(S,t)){let N=S.defaultView;if(a<0||c<0||a>N.innerWidth||c>N.innerHeight)return null;let E=Ki(b);if(E.width<=0||E.height<=0)return null;a=E.x+a*E.sx,c=E.y+c*E.sy,S=b.ownerDocument}let f=o.boundingBox,u=yg(r,new DOMRect(f.x-l,f.y-s,f.width,f.height),t),x=t.defaultView;return{...e,x:a/x.innerWidth*100,y:c+(e.isFixed?0:x.scrollY),boundingBox:{x:u.x,y:u.y+(e.isFixed?0:x.scrollY),width:u.width,height:u.height}}}function q5(e,t,n){if(!("clientX"in e)||t===n)return e;let o=Q5(t,e.clientX,e.clientY,n);return new Proxy(e,{get(r,i){if(i==="clientX")return o.x;if(i==="clientY")return o.y;let l=Reflect.get(r,i,r);return typeof l=="function"?l.bind(r):l}})}function K5(e,t){let n=new Set([e]),o=new Set,r=[],i=new Set,l=!1,s=!1,a,c=(u,x)=>{let S=b=>u.listener(q5(b,x,e));u.handlers.set(x,S),x.addEventListener(u.type,S,u.options)},f=()=>{if(l=!1,!s)return;let u=new Set,x=new Set,S=[],b=N=>{S.push(N);for(let E of N.querySelectorAll("*"))if(!E.matches("agentation-toolbar, [data-agentation-portal]")&&(E.shadowRoot&&b(E.shadowRoot),E.tagName==="IFRAME")){x.add(E);let g=vs(E);g&&!u.has(g)&&(u.add(g),b(g))}};u.add(e),b(e);for(let N of n)if(!u.has(N)){for(let E of o){let g=E.handlers.get(N);g&&N.removeEventListener(E.type,g,E.options),E.handlers.delete(N)}n.delete(N)}for(let N of u)if(!n.has(N)){n.add(N);for(let E of o)c(E,N)}for(let N of i)x.has(N)||N.removeEventListener("load",f);for(let N of x)i.has(N)||N.addEventListener("load",f);i=x,a?.disconnect(),i.forEach(N=>a?.observe(N)),r.forEach(N=>N.disconnect()),r=S.map(N=>{let E=new MutationObserver(g=>{g.some(h=>[...h.addedNodes,...h.removedNodes].some(C=>C.nodeType===1&&!C.closest("agentation-toolbar, [data-agentation-portal]")&&(C.tagName==="IFRAME"||!!C.shadowRoot||!!C.querySelector("iframe"))))&&!l&&(l=!0,queueMicrotask(f))});return E.observe(N,{childList:!0,subtree:!0}),E}),t?.()};return{start(){s||(s=!0,typeof ResizeObserver=="function"&&(a=new ResizeObserver(()=>t?.())),f())},stop(){s=!1,a?.disconnect(),a=void 0,r.forEach(u=>u.disconnect()),r=[];for(let u of i)u.removeEventListener("load",f);i.clear();for(let u of o)for(let[x,S]of u.handlers)x.removeEventListener(u.type,S,u.options);o.clear(),n.clear(),n.add(e)},addEventListener(u,x,S){let b={type:u,listener:x,options:S,handlers:new Map};o.add(b);for(let N of n)c(b,N)},removeEventListener(u,x,S){for(let b of o)if(b.type===u&&b.listener===x){for(let[N,E]of b.handlers)N.removeEventListener(u,E,b.options);o.delete(b)}},querySelectorAll(u){return[...n].flatMap(x=>[...x.querySelectorAll(u)])}}}var G_=["data-testid","data-test","data-qa","data-cy","data-component"];function ps(e,t=G_){let n={};for(let o of[...new Set(t)].slice(0,16)){if(!/^[a-zA-Z_][\w:.-]*$/.test(o))continue;let r=e.getAttribute(o);r!=null&&r.length<=500&&Object.defineProperty(n,o,{value:r,enumerable:!0})}return n}function J5(e,t=G_){return Object.entries(ps(e,t)).filter(([n,o])=>/^data-[a-z0-9_-]+$/.test(n)&&o.length<=120).slice(0,2).map(([n,o])=>`[${n}="${o.replace(/[\\"\n\r\f\0]/g,r=>r==="\\"||r==='"'?`\\${r}`:`\\${r.charCodeAt(0).toString(16)} `)}"]`).join("")}function qi(e){if(e.parentElement)return e.parentElement;let t=e.getRootNode();return X_(t)?t.host:null}function nn(e,t){let n=e;for(;n;){if(n.matches(t))return n;n=qi(n)}return null}function xg(e,t=4,n){let o=[],r=e,i=0;for(;r&&i<t;){let s=r.tagName.toLowerCase();if(s==="html"||s==="body"){o.length===0&&o.push(s);break}let a=s;if(r.id)a=`#${r.id}`;else if(r.className&&typeof r.className=="string"){let f=r.className.split(/\s+/).find(u=>u.length>2&&!u.match(/^[a-z]{1,2}$/)&&!u.match(/[A-Z0-9]{5,}/));f&&(a=`.${f.split("_")[0]}`)}a+=J5(r,n);let c=qi(r);!r.parentElement&&c&&(a=`\u27E8shadow\u27E9 ${a}`),o.unshift(a),r=c,i++}let l=zn(e.ownerDocument);return(l?xg(l,2)+" > \u27E8iframe\u27E9 ":"")+o.join(" > ")}function Z5(e){let t="";for(let n of e.childNodes)if(n.nodeType===Node.TEXT_NODE){let o=n.textContent?.trim();o&&(t+=(t?" ":"")+o)}return t}function Gi(e,t){let n=xg(e,4,t);if(e.dataset.element)return{name:e.dataset.element,path:n};let o=e.tagName.toLowerCase();if(["path","circle","rect","line","g"].includes(o)){let r=nn(e,"svg");if(r){let i=qi(r);if(i?.namespaceURI==="http://www.w3.org/1999/xhtml")return{name:`graphic in ${Gi(i).name}`,path:n}}return{name:"graphic element",path:n}}if(o==="svg"){let r=qi(e);if(r?.tagName.toLowerCase()==="button"){let i=r.textContent?.trim();return{name:i?`icon in "${i}" button`:"button icon",path:n}}return{name:"icon",path:n}}if(o==="button"){let r=e.textContent?.trim(),i=e.getAttribute("aria-label");return i?{name:`button [${i}]`,path:n}:{name:r?`button "${r.slice(0,25)}"`:"button",path:n}}if(o==="a"){let r=e.textContent?.trim(),i=e.getAttribute("href");return r?{name:`link "${r.slice(0,25)}"`,path:n}:i?{name:`link to ${i.slice(0,30)}`,path:n}:{name:"link",path:n}}if(o==="input"){let r=e.getAttribute("type")||"text",i=e.getAttribute("placeholder"),l=e.getAttribute("name");return i?{name:`input "${i}"`,path:n}:l?{name:`input [${l}]`,path:n}:{name:`${r} input`,path:n}}if(["h1","h2","h3","h4","h5","h6"].includes(o)){let r=e.textContent?.trim();return{name:r?`${o} "${r.slice(0,35)}"`:o,path:n}}if(o==="p"){let r=e.textContent?.trim();return r?{name:`paragraph: "${r.slice(0,40)}${r.length>40?"...":""}"`,path:n}:{name:"paragraph",path:n}}if(o==="span"||o==="label"){let r=e.textContent?.trim();return r&&r.length<40?{name:`"${r}"`,path:n}:{name:o,path:n}}if(o==="li"){let r=e.textContent?.trim();return r&&r.length<40?{name:`list item: "${r.slice(0,35)}"`,path:n}:{name:"list item",path:n}}if(o==="blockquote")return{name:"blockquote",path:n};if(o==="code"){let r=e.textContent?.trim();return r&&r.length<30?{name:`code: \`${r}\``,path:n}:{name:"code",path:n}}if(o==="pre")return{name:"code block",path:n};if(o==="img"){let r=e.getAttribute("alt");return{name:r?`image "${r.slice(0,30)}"`:"image",path:n}}if(o==="video")return{name:"video",path:n};if(["div","section","article","nav","header","footer","aside","main"].includes(o)){let r=e.className,i=e.getAttribute("role"),l=e.getAttribute("aria-label");if(l)return{name:`${o} [${l}]`,path:n};if(i)return{name:`${i}`,path:n};let s=Z5(e);if(s&&s.length<50)return{name:`"${s}"`,path:n};if(typeof r=="string"&&r){let a=r.split(/[\s_-]+/).map(c=>c.replace(/[A-Z0-9]{5,}.*$/,"")).filter(c=>c.length>2&&!/^[a-z]{1,2}$/.test(c)).slice(0,2);if(a.length>0)return{name:a.join(" "),path:n}}return{name:o==="div"?"container":o,path:n}}return{name:o,path:n}}function cs(e){let t=[],n=e.textContent?.trim();n&&n.length<100&&t.push(n);let o=e.previousElementSibling;if(o){let i=o.textContent?.trim();i&&i.length<50&&t.unshift(`[before: "${i.slice(0,40)}"]`)}let r=e.nextElementSibling;if(r){let i=r.textContent?.trim();i&&i.length<50&&t.push(`[after: "${i.slice(0,40)}"]`)}return t.join(" ")}function ac(e){let t=qi(e);if(!t)return"";let n=e.getRootNode(),r=(X_(n)&&e.parentElement?Array.from(e.parentElement.children):Array.from(t.children)).filter(f=>f!==e&&f.namespaceURI==="http://www.w3.org/1999/xhtml");if(r.length===0)return"";let i=r.slice(0,4).map(f=>{let u=f.tagName.toLowerCase(),x=f.className,S="";if(typeof x=="string"&&x){let b=x.split(/\s+/).map(N=>N.replace(/[_][a-zA-Z0-9]{5,}.*$/,"")).find(N=>N.length>2&&!/^[a-z]{1,2}$/.test(N));b&&(S=`.${b}`)}if(u==="button"||u==="a"){let b=f.textContent?.trim().slice(0,15);if(b)return`${u}${S} "${b}"`}return`${u}${S}`}),s=t.tagName.toLowerCase();if(typeof t.className=="string"&&t.className){let f=t.className.split(/\s+/).map(u=>u.replace(/[_][a-zA-Z0-9]{5,}.*$/,"")).find(u=>u.length>2&&!/^[a-z]{1,2}$/.test(u));f&&(s=`.${f}`)}let a=t.children.length,c=a>i.length+1?` (${a} total in ${s})`:"";return i.join(", ")+c}function ds(e){let t=e.className;return typeof t!="string"||!t?"":t.split(/\s+/).filter(o=>o.length>0).map(o=>{let r=o.match(/^([a-zA-Z][a-zA-Z0-9_-]*?)(?:_[a-zA-Z0-9]{5,})?$/);return r?r[1]:o}).filter((o,r,i)=>i.indexOf(o)===r).join(", ")}var vg=new Set(["none","normal","auto","0px","rgba(0, 0, 0, 0)","transparent","static","visible"]),e2=new Set(["p","span","h1","h2","h3","h4","h5","h6","label","li","td","th","blockquote","figcaption","caption","legend","dt","dd","pre","code","em","strong","b","i","a","time","cite","q"]),t2=new Set(["input","textarea","select"]),n2=new Set(["img","video","canvas","svg"]),o2=new Set(["div","section","article","nav","header","footer","aside","main","ul","ol","form","fieldset"]);function cc(e){if(typeof window>"u")return{};let t=(e.ownerDocument.defaultView??window).getComputedStyle(e),n={},o=e.tagName.toLowerCase(),r;e2.has(o)?r=["color","fontSize","fontWeight","fontFamily","lineHeight"]:o==="button"||o==="a"&&e.getAttribute("role")==="button"?r=["backgroundColor","color","padding","borderRadius","fontSize"]:t2.has(o)?r=["backgroundColor","color","padding","borderRadius","fontSize"]:n2.has(o)?r=["width","height","objectFit","borderRadius"]:o2.has(o)?r=["display","padding","margin","gap","backgroundColor"]:r=["color","fontSize","margin","padding","backgroundColor"];for(let i of r){let l=i.replace(/([A-Z])/g,"-$1").toLowerCase(),s=t.getPropertyValue(l);s&&!vg.has(s)&&(n[i]=s)}return n}var r2=["color","backgroundColor","borderColor","fontSize","fontWeight","fontFamily","lineHeight","letterSpacing","textAlign","width","height","padding","margin","border","borderRadius","display","position","top","right","bottom","left","zIndex","flexDirection","justifyContent","alignItems","gap","opacity","visibility","overflow","boxShadow","transform"];function dc(e){if(typeof window>"u")return"";let t=(e.ownerDocument.defaultView??window).getComputedStyle(e),n=[];for(let o of r2){let r=o.replace(/([A-Z])/g,"-$1").toLowerCase(),i=t.getPropertyValue(r);i&&!vg.has(i)&&n.push(`${r}: ${i}`)}return n.join("; ")}function i2(e){if(!e)return;let t={},n=e.split(";").map(o=>o.trim()).filter(Boolean);for(let o of n){let r=o.indexOf(":");if(r>0){let i=o.slice(0,r).trim(),l=o.slice(r+1).trim();i&&l&&(t[i]=l)}}return Object.keys(t).length>0?t:void 0}function uc(e){let t=[],n=e.getAttribute("role"),o=e.getAttribute("aria-label"),r=e.getAttribute("aria-describedby"),i=e.getAttribute("tabindex"),l=e.getAttribute("aria-hidden");return n&&t.push(`role="${n}"`),o&&t.push(`aria-label="${o}"`),r&&t.push(`aria-describedby="${r}"`),i&&t.push(`tabindex=${i}`),l==="true"&&t.push("aria-hidden"),e.matches("a, button, input, select, textarea, [tabindex]")&&t.push("focusable"),t.join(", ")}function ms(e){let t=[],n=e;for(;n&&n.tagName.toLowerCase()!=="html";){let r=n.tagName.toLowerCase(),i=r;if(n.id)i=`${r}#${n.id}`;else if(n.className&&typeof n.className=="string"){let s=n.className.split(/\s+/).map(a=>a.replace(/[_][a-zA-Z0-9]{5,}.*$/,"")).find(a=>a.length>2);s&&(i=`${r}.${s}`)}let l=qi(n);!n.parentElement&&l&&(i=`\u27E8shadow\u27E9 ${i}`),t.unshift(i),n=l}let o=zn(e.ownerDocument);return(o?ms(o)+" > \u27E8iframe\u27E9 ":"")+t.join(" > ")}var wg="agentation-toolbar, [data-agentation-root], [data-feedback-toolbar], [data-annotation-popup], [data-annotation-marker]",l2=new Set(["DIV","SPAN","SECTION","ARTICLE","MAIN","ASIDE","HEADER","FOOTER","NAV"]);function Sc(e,t){let n=document.elementFromPoint(e,t),o=new Set;for(;n&&!o.has(n);){o.add(n);let r=vs(n),i;if(r){let l=Ki(n);e=(e-l.x)/l.sx,t=(t-l.y)/l.sy,i=r.elementFromPoint?.(e,t)}else i=n.shadowRoot?.elementFromPoint?.(e,t);if(!i||i===n)break;n=i}return n}function s2(e){if(typeof e.checkVisibility=="function")return e.checkVisibility({checkOpacity:!0,checkVisibilityCSS:!0});let t=getComputedStyle(e);if(t.visibility==="hidden"||t.visibility==="collapse")return!1;let n=e;for(;n;){let o=getComputedStyle(n);if(o.opacity==="0"||o.display==="none"||o.contentVisibility==="hidden")return!1;let r=n.getRootNode();n=n.parentElement||(X_(r)?r.host:null)}return!0}function bg(e,t){let n=[],o=new Set,r=(i,l,s)=>{for(let a of i){if(o.has(a))continue;if(o.add(a),a.shadowRoot){let f=a.shadowRoot,u=f.elementsFromPoint?.(l,s)??[];r(u.length?u:[f.elementFromPoint?.(l,s)].filter(Boolean),l,s)}let c=vs(a);if(c){let f=Ki(a),u=(l-f.x)/f.sx,x=(s-f.y)/f.sy;r(c.elementsFromPoint?.(u,x)??[c.elementFromPoint?.(u,x)].filter(Boolean),u,x)}a!==a.ownerDocument.body&&a!==a.ownerDocument.documentElement&&!nn(a,wg)&&s2(a)&&n.push(a)}};return r(document.elementsFromPoint?.(e,t)??[document.elementFromPoint(e,t)].filter(Boolean),e,t),n}function _c(e,t,n){if(n.width<=0||n.height<=0)return null;let o=null,r=1/0;for(let i of bg(e,t)){let l=Wt(i),s=l.width/n.width,a=l.height/n.height;if(s<.5||s>2||a<.5||a>2)continue;let c=Math.abs(Math.log(s))+Math.abs(Math.log(a));c<r&&(o=i,r=c)}return o}function I0(e,t){let n=Sc(e,t);if(!n||nn(n,wg))return null;let o=bg(e,t);for(let l of o)if(!l2.has(l.tagName)&&!l.shadowRoot||Array.from(l.childNodes).some(s=>s.nodeType===Node.TEXT_NODE&&s.textContent?.trim()))return l;let r=null,i=1/0;for(let l of o){let s=Wt(l),a=s.width*s.height;a>0&&a<i&&(r=l,i=a)}return r}function a2(e){let t=(0,Rc.useCallback)(n=>e?(window.addEventListener("hashchange",n),window.addEventListener("popstate",n),()=>{window.removeEventListener("hashchange",n),window.removeEventListener("popstate",n)}):()=>{},[e]);return(0,Rc.useSyncExternalStore)(t,()=>window.location.pathname+(e?window.location.hash:""),()=>"/")}var fc=new Map;function c2(e,t){let n=(fc.get(e)??Promise.resolve()).then(t),o=n.then(()=>{},()=>{});return fc.set(e,o),o.then(()=>{fc.get(e)===o&&fc.delete(e)}),n}function d2(e,t,n){try{let o=new URL(e,n);return o.origin===n&&o.pathname+o.hash===t}catch{return!1}}var R0=typeof window>"u"?Ji.useEffect:Ji.useLayoutEffect;function u2(e){let[t,n]=(0,Ji.useState)(null);return R0(()=>{let o=document.createElement("div");return o.setAttribute("data-agentation-portal",""),o.style.display="contents",n(o),()=>o.remove()},[]),R0(()=>{if(!t)return;let o=e??document.body;if(o.ownerDocument!==document){console.warn("[Agentation] portalContainer belongs to another document; the toolbar will not render.");return}let r=document.activeElement;for(;r?.shadowRoot?.activeElement;)r=r.shadowRoot.activeElement;let i=r&&t.contains(document.activeElement)?r:null;typeof t.hidePopover=="function"&&t.matches(":popover-open")&&t.hidePopover(),o.appendChild(t),e&&typeof t.showPopover=="function"?(t.setAttribute("popover","manual"),t.style.cssText="position:fixed;inset:0 auto auto 0;margin:0;padding:0;border:0;background:transparent;width:0;height:0;overflow:visible;pointer-events:none",t.showPopover()):(t.removeAttribute("popover"),t.style.cssText="display:contents"),i?.focus({preventScroll:!0})},[t,e]),t}var f2=({mode:e="open",delegatesFocus:t,slotAssignment:n,host:o="div",children:r,className:i,...l})=>{let s=(0,Zi.useRef)(null),[a,c]=(0,Zi.useState)(null);return(0,Zi.useLayoutEffect)(()=>{let u=s.current;if(!u||u.shadowRoot)return;let x=u.attachShadow({mode:e,delegatesFocus:t,slotAssignment:n});c(x)},[]),_2(o,{ref:s,...l,...o.includes("-")?{class:i}:{className:i},children:a&&(0,Cg.createPortal)(r,a)})};function q_(e,t,n){let o=(0,ys.useRef)(n);(0,ys.useLayoutEffect)(()=>{o.current=n},[n]),(0,ys.useLayoutEffect)(()=>{let r=e.current;if(!t||!r)return;let i=!1,l=r.getAnimations?.()??[];return Promise.allSettled(l.map(s=>s.finished)).then(()=>{i||o.current()}),()=>{i=!0}},[e,t])}var Sg=`@charset "UTF-8";
.styles-module__popup___IhzrD svg[fill=none] {
  fill: none !important;
}
.styles-module__popup___IhzrD svg[fill=none] :not([fill]) {
  fill: none !important;
}

@keyframes styles-module__popupEnter___AuQDN {
  from {
    opacity: 0;
    transform: translateX(-50%) scale(0.95) translateY(4px);
  }
  to {
    opacity: 1;
    transform: translateX(-50%) scale(1) translateY(0);
  }
}
@keyframes styles-module__popupExit___JJKQX {
  from {
    opacity: 1;
    transform: translateX(-50%) scale(1) translateY(0);
  }
  to {
    opacity: 0;
    transform: translateX(-50%) scale(0.95) translateY(4px);
  }
}
@keyframes styles-module__shake___jdbWe {
  0%, 100% {
    transform: translateX(-50%) scale(1) translateY(0) translateX(0);
  }
  20% {
    transform: translateX(-50%) scale(1) translateY(0) translateX(-3px);
  }
  40% {
    transform: translateX(-50%) scale(1) translateY(0) translateX(3px);
  }
  60% {
    transform: translateX(-50%) scale(1) translateY(0) translateX(-2px);
  }
  80% {
    transform: translateX(-50%) scale(1) translateY(0) translateX(2px);
  }
}
.styles-module__popup___IhzrD {
  position: fixed;
  transform: translateX(-50%);
  width: 280px;
  padding: 0.75rem 1rem;
  background: #1a1a1a;
  border-radius: 16px;
  box-shadow: 0 4px 24px rgba(0, 0, 0, 0.3), 0 0 0 1px rgba(255, 255, 255, 0.08);
  z-index: 100001;
  font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
  will-change: transform, opacity;
  opacity: 0;
}
.styles-module__popup___IhzrD.styles-module__enter___L7U7N {
  animation: styles-module__popupEnter___AuQDN 0.2s cubic-bezier(0.34, 1.56, 0.64, 1) forwards;
}
.styles-module__popup___IhzrD.styles-module__entered___COX-w {
  opacity: 1;
  transform: translateX(-50%) scale(1) translateY(0);
}
.styles-module__popup___IhzrD.styles-module__exit___5eGjE {
  pointer-events: none;
  animation: styles-module__popupExit___JJKQX 0.15s ease-in forwards;
}
.styles-module__popup___IhzrD.styles-module__entered___COX-w.styles-module__shake___jdbWe {
  animation: styles-module__shake___jdbWe 0.25s ease-out;
}

.styles-module__header___wWsSi {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 0.5625rem;
}

.styles-module__element___fTV2z {
  font-size: 0.75rem;
  font-weight: 400;
  color: rgba(255, 255, 255, 0.5);
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  flex: 1;
}

.styles-module__headerToggle___WpW0b {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  background: none;
  border: none;
  padding: 0;
  cursor: pointer;
  flex: 1;
  min-width: 0;
  text-align: left;
}
.styles-module__headerToggle___WpW0b .styles-module__element___fTV2z {
  flex: 1;
}

.styles-module__chevron___ZZJlR {
  color: rgba(255, 255, 255, 0.5);
  transition: transform 0.25s cubic-bezier(0.16, 1, 0.3, 1);
  flex-shrink: 0;
}
.styles-module__chevron___ZZJlR.styles-module__expanded___2Hxgv {
  transform: rotate(90deg);
}

.styles-module__stylesWrapper___pnHgy {
  display: grid;
  grid-template-rows: 0fr;
  transition: grid-template-rows 0.3s cubic-bezier(0.16, 1, 0.3, 1);
}
.styles-module__stylesWrapper___pnHgy.styles-module__expanded___2Hxgv {
  grid-template-rows: 1fr;
}

.styles-module__stylesInner___YYZe2 {
  overflow: hidden;
}

.styles-module__stylesBlock___VfQKn {
  background: rgba(255, 255, 255, 0.05);
  border-radius: 0.375rem;
  padding: 0.5rem 0.625rem;
  margin-bottom: 0.5rem;
  font-family: ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace;
  font-size: 0.6875rem;
  line-height: 1.5;
}

.styles-module__styleLine___1YQiD {
  color: rgba(255, 255, 255, 0.85);
  word-break: break-word;
}

.styles-module__styleProperty___84L1i {
  color: #c792ea;
}

.styles-module__styleValue___q51-h {
  color: rgba(255, 255, 255, 0.85);
}

.styles-module__timestamp___Dtpsv {
  font-size: 0.625rem;
  font-weight: 500;
  color: rgba(255, 255, 255, 0.35);
  font-variant-numeric: tabular-nums;
  margin-left: 0.5rem;
  flex-shrink: 0;
}

.styles-module__quote___mcMmQ {
  font-size: 12px;
  font-style: italic;
  color: rgba(255, 255, 255, 0.6);
  margin-bottom: 0.5rem;
  padding: 0.4rem 0.5rem;
  background: rgba(255, 255, 255, 0.05);
  border-radius: 0.25rem;
  line-height: 1.45;
}

.styles-module__textarea___jrSae {
  box-sizing: border-box;
  width: 100%;
  padding: 0.5rem 0.625rem;
  font-size: 0.8125rem;
  font-family: inherit;
  background: rgba(255, 255, 255, 0.05);
  color: #fff;
  border: 1px solid rgba(255, 255, 255, 0.15);
  border-radius: 8px;
  resize: none;
  outline: none;
  transition: border-color 0.15s ease;
}
.styles-module__textarea___jrSae:focus {
  border-color: var(--agentation-color-blue);
}
.styles-module__textarea___jrSae.styles-module__green___99l3h:focus {
  border-color: var(--agentation-color-green);
}
.styles-module__textarea___jrSae::placeholder {
  color: rgba(255, 255, 255, 0.35);
}
.styles-module__textarea___jrSae::-webkit-scrollbar {
  width: 6px;
}
.styles-module__textarea___jrSae::-webkit-scrollbar-track {
  background: transparent;
}
.styles-module__textarea___jrSae::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.2);
  border-radius: 3px;
}

.styles-module__actions___D6x3f {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 0.375rem;
  margin-top: 0.75rem;
}

.styles-module__sourceAction___EabJb {
  display: block;
  max-width: 100%;
  margin: -2px 0 8px;
  padding: 2px 0;
  background: transparent;
  color: #fff;
  font: inherit;
  font-size: 11px;
  border: 0;
  cursor: pointer;
  opacity: 0.6;
  transition: opacity 0.15s ease;
}
.styles-module__sourceAction___EabJb:hover, .styles-module__sourceAction___EabJb:focus-visible {
  opacity: 1;
}
.styles-module__sourceAction___EabJb:focus-visible {
  outline: 2px solid currentColor;
  outline-offset: 3px;
}

.styles-module__light___6AaSQ .styles-module__sourceAction___EabJb {
  color: #111;
}

.styles-module__cancel___hRjnL,
.styles-module__submit___K-mIR,
.styles-module__deleteButton___4VuAE {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 1.875rem;
  padding: 0.375rem 0.875rem;
  font-size: 0.75rem;
  font-weight: 500;
  border-radius: 1rem;
  border: none;
  cursor: pointer;
  transition: background-color 0.15s ease, color 0.15s ease, opacity 0.15s ease;
}

.styles-module__cancel___hRjnL {
  background: transparent;
  color: rgba(255, 255, 255, 0.5);
}
.styles-module__cancel___hRjnL:hover {
  background: rgba(255, 255, 255, 0.1);
  color: rgba(255, 255, 255, 0.8);
}

.styles-module__submit___K-mIR {
  color: white;
}
.styles-module__submit___K-mIR:hover:not(:disabled) {
  filter: brightness(0.9);
}
.styles-module__submit___K-mIR:disabled {
  cursor: not-allowed;
}

.styles-module__deleteWrapper___oSjdo {
  display: flex;
  margin-right: auto;
}

.styles-module__deleteButton___4VuAE {
  background: transparent;
  color: rgba(255, 255, 255, 0.4);
  transition: background-color 0.15s ease, color 0.15s ease, transform 0.1s ease;
}
.styles-module__deleteButton___4VuAE:hover {
  background-color: color-mix(in srgb, var(--agentation-color-red) 25%, transparent);
  color: var(--agentation-color-red);
}
.styles-module__deleteButton___4VuAE:active {
  transform: scale(0.92);
}

.styles-module__light___6AaSQ.styles-module__popup___IhzrD {
  background: #fff;
  box-shadow: 0 4px 24px rgba(0, 0, 0, 0.12), 0 0 0 1px rgba(0, 0, 0, 0.06);
}
.styles-module__light___6AaSQ .styles-module__element___fTV2z {
  color: rgba(0, 0, 0, 0.6);
}
.styles-module__light___6AaSQ .styles-module__timestamp___Dtpsv {
  color: rgba(0, 0, 0, 0.4);
}
.styles-module__light___6AaSQ .styles-module__chevron___ZZJlR {
  color: rgba(0, 0, 0, 0.4);
}
.styles-module__light___6AaSQ .styles-module__stylesBlock___VfQKn {
  background: rgba(0, 0, 0, 0.03);
}
.styles-module__light___6AaSQ .styles-module__styleLine___1YQiD {
  color: rgba(0, 0, 0, 0.75);
}
.styles-module__light___6AaSQ .styles-module__styleProperty___84L1i {
  color: #7c3aed;
}
.styles-module__light___6AaSQ .styles-module__styleValue___q51-h {
  color: rgba(0, 0, 0, 0.75);
}
.styles-module__light___6AaSQ .styles-module__quote___mcMmQ {
  color: rgba(0, 0, 0, 0.55);
  background: rgba(0, 0, 0, 0.04);
}
.styles-module__light___6AaSQ .styles-module__textarea___jrSae {
  background: rgba(0, 0, 0, 0.03);
  color: #1a1a1a;
  border-color: rgba(0, 0, 0, 0.12);
}
.styles-module__light___6AaSQ .styles-module__textarea___jrSae::placeholder {
  color: rgba(0, 0, 0, 0.4);
}
.styles-module__light___6AaSQ .styles-module__textarea___jrSae::-webkit-scrollbar-thumb {
  background: rgba(0, 0, 0, 0.15);
}
.styles-module__light___6AaSQ .styles-module__cancel___hRjnL {
  color: rgba(0, 0, 0, 0.5);
}
.styles-module__light___6AaSQ .styles-module__cancel___hRjnL:hover {
  background: rgba(0, 0, 0, 0.06);
  color: rgba(0, 0, 0, 0.75);
}
.styles-module__light___6AaSQ .styles-module__deleteButton___4VuAE {
  color: rgba(0, 0, 0, 0.4);
}
.styles-module__light___6AaSQ .styles-module__deleteButton___4VuAE:hover {
  background-color: color-mix(in srgb, var(--agentation-color-red) 25%, transparent);
  color: var(--agentation-color-red);
}

@media (prefers-reduced-motion: reduce) {
  .styles-module__popup___IhzrD.styles-module__enter___L7U7N, .styles-module__popup___IhzrD.styles-module__exit___5eGjE {
    animation-duration: 1ms;
    animation-delay: 0ms !important;
  }
}
.styles-module__sharedForm___8GvQl {
  --card-motion: 200ms cubic-bezier(0.2, 0.8, 0.2, 1);
  padding: 0.75rem 1rem;
  transition: padding var(--card-motion);
}
.styles-module__sharedForm___8GvQl .styles-module__header___wWsSi {
  transition: margin-bottom var(--card-motion);
}
.styles-module__sharedForm___8GvQl .styles-module__element___fTV2z {
  font-style: italic;
  color: rgba(255, 255, 255, 0.6);
}
.styles-module__sharedForm___8GvQl .styles-module__previewExcerpt___DCOIL {
  display: none;
}
.styles-module__sharedForm___8GvQl .styles-module__headerToggle___WpW0b .styles-module__chevron___ZZJlR {
  opacity: 1;
  margin-left: 0;
  transition: margin-left var(--card-motion), opacity 100ms ease-out, transform var(--card-motion);
}
.styles-module__sharedForm___8GvQl .styles-module__sharedNote___OYVi5 {
  position: relative;
  height: var(--editor-field-height, 57px);
  border-radius: 8px;
  overflow: clip;
  transition: height var(--card-motion);
}
.styles-module__sharedForm___8GvQl .styles-module__sharedNote___OYVi5::before {
  content: "";
  position: absolute;
  inset: 0;
  border: 1px solid var(--field-border, rgba(255, 255, 255, 0.15));
  border-radius: inherit;
  background: rgba(255, 255, 255, 0.05);
  pointer-events: none;
  transition: opacity var(--card-motion), border-color var(--card-motion);
}
.styles-module__sharedForm___8GvQl .styles-module__sharedNoteContent___6q3KD {
  position: relative;
  width: 100%;
  height: 100%;
  overflow: clip;
  transition: width var(--card-motion);
}
.styles-module__sharedForm___8GvQl .styles-module__sharedNote___OYVi5[data-truncated]::after {
  content: "\u2026";
  position: absolute;
  right: 0;
  top: 0;
  color: #fff;
  font-size: 13px;
  line-height: 1.4;
  opacity: 0;
  pointer-events: none;
  transition: opacity 80ms ease-out;
}
.styles-module__sharedForm___8GvQl .styles-module__textarea___jrSae {
  display: block;
  width: calc(280px - 2rem);
  max-width: calc(100vw - 24px - 2rem);
  margin: 0;
  background: transparent !important;
  border-color: transparent !important;
  transform: translate(0, 0);
  transition: transform var(--card-motion), color var(--card-motion);
}
.styles-module__sharedForm___8GvQl .styles-module__sharedExtra___RUBKC, .styles-module__sharedForm___8GvQl .styles-module__sharedActions___6Glpl {
  display: grid;
  grid-template-rows: 1fr;
  opacity: 1;
  transition: grid-template-rows var(--card-motion), opacity 80ms ease-out;
}
.styles-module__sharedForm___8GvQl .styles-module__sharedActions___6Glpl {
  transition-delay: 0ms, 120ms;
}
.styles-module__sharedForm___8GvQl .styles-module__sharedExtraInner___EuUh4 {
  min-height: 0;
  overflow: hidden;
}
.styles-module__sharedForm___8GvQl .styles-module__actions___D6x3f {
  min-height: 0;
  overflow: hidden;
  transition: margin-top var(--card-motion);
}
.styles-module__sharedForm___8GvQl[data-preview] {
  padding: 8px 12px;
}
.styles-module__sharedForm___8GvQl[data-preview] .styles-module__header___wWsSi {
  margin-bottom: 5px;
}
.styles-module__sharedForm___8GvQl[data-preview] .styles-module__previewExcerpt___DCOIL {
  display: inline;
}
.styles-module__sharedForm___8GvQl[data-preview] .styles-module__element___fTV2z {
  line-height: 1.4;
}
.styles-module__sharedForm___8GvQl[data-preview] .styles-module__chevron___ZZJlR {
  opacity: 0;
  margin-left: -18px;
}
.styles-module__sharedForm___8GvQl[data-preview] .styles-module__sharedNote___OYVi5 {
  height: 20.2px;
  border-radius: 0;
}
.styles-module__sharedForm___8GvQl[data-preview] .styles-module__sharedNote___OYVi5::before {
  opacity: 0;
}
.styles-module__sharedForm___8GvQl[data-preview] .styles-module__textarea___jrSae {
  transform: translate(-11px, calc(-9px + (1.4em - 1lh) / 2));
  color: #fff;
  overflow: hidden;
  cursor: default;
}
.styles-module__sharedForm___8GvQl[data-preview] .styles-module__sharedExtra___RUBKC, .styles-module__sharedForm___8GvQl[data-preview] .styles-module__sharedActions___6Glpl {
  grid-template-rows: 0fr;
  opacity: 0;
  transition-delay: 0ms;
}
.styles-module__sharedForm___8GvQl[data-preview] .styles-module__actions___D6x3f {
  margin-top: 0;
}
.styles-module__sharedForm___8GvQl[data-preview] .styles-module__stylesWrapper___pnHgy {
  grid-template-rows: 0fr;
}
.styles-module__sharedForm___8GvQl[data-preview] .styles-module__sharedNote___OYVi5[data-truncated] .styles-module__sharedNoteContent___6q3KD {
  width: calc(100% - 12px);
}
.styles-module__sharedForm___8GvQl[data-preview] .styles-module__sharedNote___OYVi5[data-truncated]::after {
  opacity: 1;
}

.styles-module__light___6AaSQ .styles-module__sharedForm___8GvQl .styles-module__sharedNote___OYVi5::before {
  border-color: var(--field-border, rgba(0, 0, 0, 0.12));
  background: rgba(0, 0, 0, 0.03);
}

.styles-module__light___6AaSQ .styles-module__sharedForm___8GvQl .styles-module__element___fTV2z {
  color: rgba(0, 0, 0, 0.5);
}

.styles-module__light___6AaSQ .styles-module__sharedForm___8GvQl[data-preview] .styles-module__textarea___jrSae, .styles-module__light___6AaSQ .styles-module__sharedForm___8GvQl[data-preview] .styles-module__sharedNote___OYVi5::after {
  color: rgba(0, 0, 0, 0.85);
}

@media (prefers-reduced-motion: reduce) {
  .styles-module__sharedForm___8GvQl {
    --card-motion: 1ms linear;
  }
  .styles-module__sharedForm___8GvQl .styles-module__sharedActions___6Glpl, .styles-module__sharedForm___8GvQl .styles-module__headerToggle___WpW0b .styles-module__chevron___ZZJlR {
    transition: opacity 100ms ease-out;
    transition-delay: 0ms;
  }
}`,Be={popup:"styles-module__popup___IhzrD",enter:"styles-module__enter___L7U7N",popupEnter:"styles-module__popupEnter___AuQDN",entered:"styles-module__entered___COX-w",exit:"styles-module__exit___5eGjE",popupExit:"styles-module__popupExit___JJKQX",shake:"styles-module__shake___jdbWe",header:"styles-module__header___wWsSi",element:"styles-module__element___fTV2z",headerToggle:"styles-module__headerToggle___WpW0b",chevron:"styles-module__chevron___ZZJlR",expanded:"styles-module__expanded___2Hxgv",stylesWrapper:"styles-module__stylesWrapper___pnHgy",stylesInner:"styles-module__stylesInner___YYZe2",stylesBlock:"styles-module__stylesBlock___VfQKn",styleLine:"styles-module__styleLine___1YQiD",styleProperty:"styles-module__styleProperty___84L1i",styleValue:"styles-module__styleValue___q51-h",timestamp:"styles-module__timestamp___Dtpsv",quote:"styles-module__quote___mcMmQ",textarea:"styles-module__textarea___jrSae",green:"styles-module__green___99l3h",actions:"styles-module__actions___D6x3f",sourceAction:"styles-module__sourceAction___EabJb",light:"styles-module__light___6AaSQ",cancel:"styles-module__cancel___hRjnL",submit:"styles-module__submit___K-mIR",deleteButton:"styles-module__deleteButton___4VuAE",deleteWrapper:"styles-module__deleteWrapper___oSjdo",sharedForm:"styles-module__sharedForm___8GvQl",previewExcerpt:"styles-module__previewExcerpt___DCOIL",sharedNote:"styles-module__sharedNote___OYVi5",sharedNoteContent:"styles-module__sharedNoteContent___6q3KD",sharedExtra:"styles-module__sharedExtra___RUBKC",sharedActions:"styles-module__sharedActions___6Glpl",sharedExtraInner:"styles-module__sharedExtraInner___EuUh4"},L_="data-agentation-styles";function Eg(e,t,n){if(!n||!e)return;let o=e.nodeType===9?e.head:e.nodeType===11?e:null;if(!o||typeof o.querySelector!="function"||o.querySelector(`style[${L_}~="toolbar"], style[${L_}~="${t}"]`))return;let i=(e.nodeType===9?e:e.ownerDocument).createElement("style");i.setAttribute(L_,t),i.textContent=n,o.appendChild(i)}function K_(e,t){return(0,Mg.useCallback)(n=>{n&&Eg(n.getRootNode(),e,t)},[e,t])}function $0(e){if(!e)return;let t=n=>n.stopImmediatePropagation();document.addEventListener("focusin",t,!0),document.addEventListener("focusout",t,!0);try{e.focus({preventScroll:!0})}finally{document.removeEventListener("focusin",t,!0),document.removeEventListener("focusout",t,!0)}}var Lg=(0,hn.forwardRef)(function({element:t,timestamp:n,selectedText:o,placeholder:r="What should change?",initialValue:i="",submitLabel:l="Add",onSubmit:s,onCancel:a,onDelete:c,onOpenSource:f,allowEmpty:u=!1,accentColor:x="#3c82f7",computedStyles:S,disabled:b=!1,preview:N=!1,resetOnPreview:E=!0,variant:g="popup"},v){let h=g==="card",[C,U]=(0,hn.useState)(i),[V,B]=(0,hn.useState)(!1),[q,F]=(0,hn.useState)(!1),K=(0,hn.useRef)(null),se=(0,hn.useRef)(null),Z=o?` "${o.slice(0,30)}${o.length>30?"...":""}"`:"";(0,hn.useLayoutEffect)(()=>{let ae=se.current,pe=K.current;if(!h||!ae||!pe)return;let kt=()=>{ae.style.setProperty("--editor-field-height",`${pe.offsetHeight}px`)};if(kt(),"CanvasRenderingContext2D"in window){let qe=document.createElement("canvas").getContext("2d");if(qe){let Ae=getComputedStyle(pe).fontFamily;qe.font=`13px ${Ae}`;let _t=qe.measureText(i.replace(/\s+/g," ")).width;qe.font=`italic 12px ${Ae}`;let qt=qe.measureText(t+Z).width,wn=Math.min(200,Math.max(120,Math.ceil(Math.max(_t,qt))+24));ae.closest("[data-annotation-card]")?.style.setProperty("--preview-width",`${wn}px`);let Re=ae.querySelector("[data-shared-note]");Re&&Re.toggleAttribute("data-truncated",_t>wn-24)}}let Oe=typeof ResizeObserver<"u"?new ResizeObserver(kt):null;return Oe?.observe(pe),()=>Oe?.disconnect()},[h,t,i,Z]),(0,hn.useLayoutEffect)(()=>{N&&E&&(U(i),F(!1),K.current&&(K.current.scrollTop=0,K.current.scrollLeft=0))},[N,E,i]),(0,hn.useImperativeHandle)(v,()=>({focus(){let ae=K.current;$0(ae),ae&&(ae.selectionStart=ae.selectionEnd=ae.value.length,ae.scrollTop=h?0:ae.scrollHeight)}}),[h]);let fe=(0,hn.useCallback)(()=>{b||!C.trim()&&!u||s(C.trim())},[b,C,u,s]),oe=ae=>{ae.stopPropagation(),!ae.nativeEvent.isComposing&&(ae.key==="Enter"&&!ae.shiftKey&&(ae.preventDefault(),fe()),ae.key==="Escape"&&a())};return br("div",{ref:se,className:h?Be.sharedForm:void 0,style:h?void 0:{display:"contents"},"data-annotation-editor":!0,"data-preview":N||void 0,children:[br("div",{className:Be.header,"data-editor-heading":!0,children:[S&&Object.keys(S).length>0?br("button",{className:Be.headerToggle,onClick:()=>{let ae=q;F(!q),ae&&nt(()=>$0(K.current),0)},type:"button",children:[$t("svg",{className:`${Be.chevron} ${q?Be.expanded:""}`,width:"14",height:"14",viewBox:"0 0 14 14",fill:"none",xmlns:"http://www.w3.org/2000/svg",children:$t("path",{d:"M5.5 10.25L9 7.25L5.75 4",stroke:"currentColor",strokeWidth:"1.5",strokeLinecap:"round",strokeLinejoin:"round"})}),br("span",{className:Be.element,children:[t,h&&Z&&$t("span",{className:Be.previewExcerpt,children:Z})]})]}):br("span",{className:Be.element,children:[t,h&&Z&&$t("span",{className:Be.previewExcerpt,children:Z})]}),n&&$t("span",{className:Be.timestamp,children:n})]}),f&&$t("div",{className:h?Be.sharedExtra:void 0,style:h?void 0:{display:"contents"},children:$t("div",{className:h?Be.sharedExtraInner:void 0,style:h?void 0:{display:"contents"},children:$t("button",{type:"button",className:Be.sourceAction,onClick:f,children:"Open in editor"})})}),S&&Object.keys(S).length>0&&$t("div",{className:`${Be.stylesWrapper} ${q?Be.expanded:""}`,children:$t("div",{className:Be.stylesInner,children:$t("div",{className:Be.stylesBlock,children:Object.entries(S).map(([ae,pe])=>br("div",{className:Be.styleLine,children:[$t("span",{className:Be.styleProperty,children:ae.replace(/([A-Z])/g,"-$1").toLowerCase()}),": ",$t("span",{className:Be.styleValue,children:pe}),";"]},ae))})})}),o&&$t("div",{className:h?Be.sharedExtra:void 0,style:h?void 0:{display:"contents"},children:$t("div",{className:h?Be.sharedExtraInner:void 0,style:h?void 0:{display:"contents"},children:br("div",{className:Be.quote,children:["\u201C",o.slice(0,80),o.length>80?"...":"","\u201D"]})})}),$t("div",{"data-shared-note":!0,className:h?Be.sharedNote:void 0,style:h?{"--field-border":V?x:void 0}:{display:"contents"},children:$t("div",{className:h?Be.sharedNoteContent:void 0,style:h?void 0:{display:"contents"},children:$t("textarea",{ref:K,className:Be.textarea,readOnly:N,"aria-hidden":N,style:h?void 0:{borderColor:V?x:void 0},placeholder:r,value:N?C.replace(/\s+/g," "):C,onChange:ae=>U(ae.target.value),onFocus:()=>B(!0),onBlur:()=>B(!1),rows:2,onKeyDown:oe})})}),$t("div",{"data-editor-actions":!0,className:h?Be.sharedActions:void 0,style:h?void 0:{display:"contents"},children:br("div",{className:Be.actions,children:[c&&$t("div",{className:Be.deleteWrapper,children:$t("button",{className:Be.deleteButton,onClick:c,type:"button","aria-label":"Delete annotation",children:"Delete"})}),$t("button",{className:Be.cancel,onClick:a,children:"Cancel"}),$t("button",{className:Be.submit,style:{backgroundColor:x,opacity:C.trim()||u?1:.4},onClick:fe,disabled:b||!C.trim()&&!u,children:l})]})})]})}),J_=(0,Xt.forwardRef)(function({element:t,timestamp:n,selectedText:o,placeholder:r="What should change?",initialValue:i="",submitLabel:l="Add",onSubmit:s,onCancel:a,onDelete:c,onOpenSource:f,allowEmpty:u=!1,style:x,accentColor:S="#3c82f7",isExiting:b=!1,onExitComplete:N,lightMode:E=!1,computedStyles:g},v){let[h,C]=(0,Xt.useState)(!1),[U,V]=(0,Xt.useState)("initial"),B=(0,Xt.useRef)(null),q=(0,Xt.useRef)(null);(0,Xt.useEffect)(()=>{Eg(q.current?.getRootNode(),"annotation-popup",Sg)},[]);let F=(0,Xt.useRef)(null);(0,Xt.useEffect)(()=>{let oe=nt(()=>{V(ae=>ae==="initial"?"enter":ae)},0);return()=>{clearTimeout(oe),F.current&&clearTimeout(F.current)}},[]),(0,Xt.useEffect)(()=>{if(b)return;let oe=nt(()=>B.current?.focus(),50);return()=>clearTimeout(oe)},[b]);let K=(0,Xt.useCallback)(()=>{F.current&&clearTimeout(F.current),C(!0),F.current=nt(()=>{C(!1),B.current?.focus()},250)},[]);(0,Xt.useImperativeHandle)(v,()=>({shake:K}),[K]);let se=(0,Xt.useCallback)(()=>{if(N){a();return}V("exit")},[a,N]),Z=b?"exit":U;q_(q,Z==="exit",()=>{b?N?.():a()});let fe=[Be.popup,E?Be.light:"",Z==="enter"?Be.enter:"",Z==="entered"?Be.entered:"",Z==="exit"?Be.exit:"",h&&Z!=="exit"?Be.shake:""].filter(Boolean).join(" ");return T0("div",{ref:q,className:fe,"data-annotation-popup":!0,style:x,onAnimationEnd:oe=>{oe.target===oe.currentTarget&&oe.animationName.includes("popupEnter")&&!b&&V("entered")},onKeyDownCapture:oe=>{oe.key!=="Escape"||oe.nativeEvent.isComposing||(oe.preventDefault(),oe.stopPropagation(),se())},onClick:oe=>oe.stopPropagation(),children:T0(Lg,{ref:B,element:t,timestamp:n,selectedText:o,placeholder:r,initialValue:i,submitLabel:l,onSubmit:s,onCancel:se,onDelete:c,onOpenSource:f,allowEmpty:u,accentColor:S,computedStyles:g,disabled:Z==="exit"})})}),$c=`.icon-transitions-module__iconState___uqK9J {
  transition: opacity 0.2s ease, transform 0.2s ease;
  transform-origin: center;
}

.icon-transitions-module__iconStateFast___HxlMm {
  transition: opacity 0.15s ease, transform 0.15s ease;
  transform-origin: center;
}

.icon-transitions-module__iconFade___nPwXg {
  transition: opacity 0.2s ease;
}

.icon-transitions-module__iconFadeFast___Ofb2t {
  transition: opacity 0.15s ease;
}

.icon-transitions-module__visible___PlHsU {
  opacity: 1 !important;
}

.icon-transitions-module__visibleScaled___8Qog- {
  opacity: 1 !important;
  transform: scale(1);
}

.icon-transitions-module__hidden___ETykt {
  opacity: 0 !important;
}

.icon-transitions-module__hiddenScaled___JXn-m {
  opacity: 0 !important;
  transform: scale(0.8);
}

.icon-transitions-module__sending___uaLN- {
  opacity: 0.5 !important;
  transform: scale(0.8);
}`,pt={iconState:"icon-transitions-module__iconState___uqK9J",iconStateFast:"icon-transitions-module__iconStateFast___HxlMm",iconFade:"icon-transitions-module__iconFade___nPwXg",iconFadeFast:"icon-transitions-module__iconFadeFast___Ofb2t",visible:"icon-transitions-module__visible___PlHsU",visibleScaled:"icon-transitions-module__visibleScaled___8Qog-",hidden:"icon-transitions-module__hidden___ETykt",hiddenScaled:"icon-transitions-module__hiddenScaled___JXn-m",sending:"icon-transitions-module__sending___uaLN-"};var h2=({size:e=16})=>ve("svg",{width:e,height:e,viewBox:"0 0 16 16",fill:"none",children:ve("path",{d:"M8 3v10M3 8h10",stroke:"currentColor",strokeWidth:"1.5",strokeLinecap:"round"})});var p2=({size:e=20,...t})=>on("svg",{width:e,height:e,viewBox:"0 0 20 20",fill:"none",xmlns:"http://www.w3.org/2000/svg",...t,children:[ve("circle",{cx:"10",cy:"10",r:"5.375",stroke:"currentColor",strokeWidth:"1.25"}),ve("path",{d:"M8.5 8.5C8.73 7.85 9.31 7.49 10 7.5C10.86 7.51 11.5 8.13 11.5 9C11.5 10.08 10 10.5 10 10.5V10.75",stroke:"currentColor",strokeWidth:"1.25",strokeLinecap:"round",strokeLinejoin:"round"}),ve("circle",{cx:"10",cy:"12.625",r:"0.625",fill:"currentColor"})]});var m2=({size:e=24,copied:t=!1,tint:n})=>on("svg",{ref:K_("icon-transitions",$c),width:e,height:e,viewBox:"0 0 24 24",fill:"none",style:n?{color:n,transition:"color 0.3s ease"}:void 0,children:[on("g",{className:`${pt.iconState} ${t?pt.hiddenScaled:pt.visibleScaled}`,children:[ve("path",{d:"M4.75 11.25C4.75 10.4216 5.42157 9.75 6.25 9.75H12.75C13.5784 9.75 14.25 10.4216 14.25 11.25V17.75C14.25 18.5784 13.5784 19.25 12.75 19.25H6.25C5.42157 19.25 4.75 18.5784 4.75 17.75V11.25Z",stroke:"currentColor",strokeWidth:"1.5"}),ve("path",{d:"M17.25 14.25H17.75C18.5784 14.25 19.25 13.5784 19.25 12.75V6.25C19.25 5.42157 18.5784 4.75 17.75 4.75H11.25C10.4216 4.75 9.75 5.42157 9.75 6.25V6.75",stroke:"currentColor",strokeWidth:"1.5",strokeLinecap:"round"})]}),on("g",{className:`${pt.iconState} ${t?pt.visibleScaled:pt.hiddenScaled}`,children:[ve("path",{d:"M12 20C7.58172 20 4 16.4182 4 12C4 7.58172 7.58172 4 12 4C16.4182 4 20 7.58172 20 12C20 16.4182 16.4182 20 12 20Z",stroke:"var(--agentation-color-green)",strokeWidth:"1.5",strokeLinecap:"round",strokeLinejoin:"round"}),ve("path",{d:"M15 10L11 14.25L9.25 12.25",stroke:"var(--agentation-color-green)",strokeWidth:"1.5",strokeLinecap:"round",strokeLinejoin:"round"})]})]}),g2=({size:e=24,state:t="idle"})=>{let n=t==="idle",o=t==="sent",r=t==="failed",i=t==="sending";return on("svg",{width:e,height:e,viewBox:"0 0 24 24",fill:"none",children:[ve("g",{className:`${pt.iconStateFast} ${n?pt.visibleScaled:i?pt.sending:pt.hiddenScaled}`,children:ve("path",{d:"M9.875 14.125L12.3506 19.6951C12.7184 20.5227 13.9091 20.4741 14.2083 19.6193L18.8139 6.46032C19.0907 5.6695 18.3305 4.90933 17.5397 5.18611L4.38072 9.79174C3.52589 10.0909 3.47731 11.2816 4.30494 11.6494L9.875 14.125ZM9.875 14.125L13.375 10.625",stroke:"currentColor",strokeWidth:"1.5",strokeLinecap:"round",strokeLinejoin:"round"})}),on("g",{className:`${pt.iconStateFast} ${o?pt.visibleScaled:pt.hiddenScaled}`,children:[ve("path",{d:"M12 20C7.58172 20 4 16.4182 4 12C4 7.58172 7.58172 4 12 4C16.4182 4 20 7.58172 20 12C20 16.4182 16.4182 20 12 20Z",stroke:"var(--agentation-color-green)",strokeWidth:"1.5",strokeLinecap:"round",strokeLinejoin:"round"}),ve("path",{d:"M15 10L11 14.25L9.25 12.25",stroke:"var(--agentation-color-green)",strokeWidth:"1.5",strokeLinecap:"round",strokeLinejoin:"round"})]}),on("g",{className:`${pt.iconStateFast} ${r?pt.visibleScaled:pt.hiddenScaled}`,children:[ve("path",{d:"M12 20C7.58172 20 4 16.4182 4 12C4 7.58172 7.58172 4 12 4C16.4182 4 20 7.58172 20 12C20 16.4182 16.4182 20 12 20Z",stroke:"var(--agentation-color-red)",strokeWidth:"1.5",strokeLinecap:"round",strokeLinejoin:"round"}),ve("path",{d:"M12 8V12",stroke:"var(--agentation-color-red)",strokeWidth:"1.5",strokeLinecap:"round"}),ve("circle",{cx:"12",cy:"15",r:"0.5",fill:"var(--agentation-color-red)",stroke:"var(--agentation-color-red)",strokeWidth:"1"})]})]})};var y2=({size:e=24,isOpen:t=!0})=>on("svg",{ref:K_("icon-transitions",$c),width:e,height:e,viewBox:"0 0 24 24",fill:"none",children:[on("g",{className:`${pt.iconFade} ${t?pt.visible:pt.hidden}`,children:[ve("path",{d:"M3.91752 12.7539C3.65127 12.2996 3.65037 11.7515 3.9149 11.2962C4.9042 9.59346 7.72688 5.49994 12 5.49994C16.2731 5.49994 19.0958 9.59346 20.0851 11.2962C20.3496 11.7515 20.3487 12.2996 20.0825 12.7539C19.0908 14.4459 16.2694 18.4999 12 18.4999C7.73064 18.4999 4.90918 14.4459 3.91752 12.7539Z",stroke:"currentColor",strokeWidth:"1.5",strokeLinecap:"round",strokeLinejoin:"round"}),ve("path",{d:"M12 14.8261C13.5608 14.8261 14.8261 13.5608 14.8261 12C14.8261 10.4392 13.5608 9.17392 12 9.17392C10.4392 9.17392 9.17391 10.4392 9.17391 12C9.17391 13.5608 10.4392 14.8261 12 14.8261Z",stroke:"currentColor",strokeWidth:"1.5",strokeLinecap:"round",strokeLinejoin:"round"})]}),on("g",{className:`${pt.iconFade} ${t?pt.hidden:pt.visible}`,children:[ve("path",{d:"M18.6025 9.28503C18.9174 8.9701 19.4364 8.99481 19.7015 9.35271C20.1484 9.95606 20.4943 10.507 20.7342 10.9199C21.134 11.6086 21.1329 12.4454 20.7303 13.1328C20.2144 14.013 19.2151 15.5225 17.7723 16.8193C16.3293 18.1162 14.3852 19.2497 12.0008 19.25C11.4192 19.25 10.8638 19.1823 10.3355 19.0613C9.77966 18.934 9.63498 18.2525 10.0382 17.8493C10.2412 17.6463 10.5374 17.573 10.8188 17.6302C11.1993 17.7076 11.5935 17.75 12.0008 17.75C13.8848 17.7497 15.4867 16.8568 16.7693 15.7041C18.0522 14.5511 18.9606 13.1867 19.4363 12.375C19.5656 12.1543 19.5659 11.8943 19.4373 11.6729C19.2235 11.3049 18.921 10.8242 18.5364 10.3003C18.3085 9.98991 18.3302 9.5573 18.6025 9.28503ZM12.0008 4.75C12.5814 4.75006 13.1358 4.81803 13.6632 4.93953C14.2182 5.06741 14.362 5.74812 13.9593 6.15091C13.7558 6.35435 13.4589 6.42748 13.1771 6.36984C12.7983 6.29239 12.4061 6.25006 12.0008 6.25C10.1167 6.25 8.51415 7.15145 7.23028 8.31543C5.94678 9.47919 5.03918 10.8555 4.56426 11.6729C4.43551 11.8945 4.43582 12.1542 4.56524 12.375C4.77587 12.7343 5.07189 13.2012 5.44718 13.7105C5.67623 14.0213 5.65493 14.4552 5.38193 14.7282C5.0671 15.0431 4.54833 15.0189 4.28292 14.6614C3.84652 14.0736 3.50813 13.5369 3.27129 13.1328C2.86831 12.4451 2.86717 11.6088 3.26739 10.9199C3.78185 10.0345 4.77959 8.51239 6.22247 7.2041C7.66547 5.89584 9.61202 4.75 12.0008 4.75Z",fill:"currentColor"}),ve("path",{d:"M5 19L19 5",stroke:"currentColor",strokeWidth:"1.5",strokeLinecap:"round"})]})]}),x2=({size:e=24,isPaused:t=!1})=>on("svg",{ref:K_("icon-transitions",$c),width:e,height:e,viewBox:"0 0 24 24",fill:"none",children:[on("g",{className:`${pt.iconFadeFast} ${t?pt.hidden:pt.visible}`,children:[ve("path",{d:"M8 6L8 18",stroke:"currentColor",strokeWidth:"1.5",strokeLinecap:"round"}),ve("path",{d:"M16 18L16 6",stroke:"currentColor",strokeWidth:"1.5",strokeLinecap:"round"})]}),ve("path",{className:`${pt.iconFadeFast} ${t?pt.visible:pt.hidden}`,d:"M17.75 10.701C18.75 11.2783 18.75 12.7217 17.75 13.299L8.75 18.4952C7.75 19.0725 6.5 18.3509 6.5 17.1962L6.5 6.80384C6.5 5.64914 7.75 4.92746 8.75 5.50481L17.75 10.701Z",stroke:"currentColor",strokeWidth:"1.5"})]});var v2=({size:e=16})=>on("svg",{width:e,height:e,viewBox:"0 0 24 24",fill:"none",children:[ve("path",{d:"M10.6504 5.81117C10.9939 4.39628 13.0061 4.39628 13.3496 5.81117C13.5715 6.72517 14.6187 7.15891 15.4219 6.66952C16.6652 5.91193 18.0881 7.33479 17.3305 8.57815C16.8411 9.38134 17.2748 10.4285 18.1888 10.6504C19.6037 10.9939 19.6037 13.0061 18.1888 13.3496C17.2748 13.5715 16.8411 14.6187 17.3305 15.4219C18.0881 16.6652 16.6652 18.0881 15.4219 17.3305C14.6187 16.8411 13.5715 17.2748 13.3496 18.1888C13.0061 19.6037 10.9939 19.6037 10.6504 18.1888C10.4285 17.2748 9.38135 16.8411 8.57815 17.3305C7.33479 18.0881 5.91193 16.6652 6.66952 15.4219C7.15891 14.6187 6.72517 13.5715 5.81117 13.3496C4.39628 13.0061 4.39628 10.9939 5.81117 10.6504C6.72517 10.4285 7.15891 9.38134 6.66952 8.57815C5.91193 7.33479 7.33479 5.91192 8.57815 6.66952C9.38135 7.15891 10.4285 6.72517 10.6504 5.81117Z",stroke:"currentColor",strokeWidth:"1.5",strokeLinecap:"round",strokeLinejoin:"round"}),ve("circle",{cx:"12",cy:"12",r:"2.5",stroke:"currentColor",strokeWidth:"1.5"})]});var w2=({size:e=16})=>ve("svg",{width:e,height:e,viewBox:"0 0 24 24",fill:"none",children:ve("path",{d:"M13.5 4C14.7426 4 15.75 5.00736 15.75 6.25V7H18.5C18.9142 7 19.25 7.33579 19.25 7.75C19.25 8.16421 18.9142 8.5 18.5 8.5H17.9678L17.6328 16.2217C17.61 16.7475 17.5912 17.1861 17.5469 17.543C17.5015 17.9087 17.4225 18.2506 17.2461 18.5723C16.9747 19.0671 16.5579 19.4671 16.0518 19.7168C15.7227 19.8791 15.3772 19.9422 15.0098 19.9717C14.6514 20.0004 14.2126 20 13.6865 20H10.3135C9.78735 20 9.34856 20.0004 8.99023 19.9717C8.62278 19.9422 8.27729 19.8791 7.94824 19.7168C7.44205 19.4671 7.02532 19.0671 6.75391 18.5723C6.57751 18.2506 6.49853 17.9087 6.45312 17.543C6.40883 17.1861 6.39005 16.7475 6.36719 16.2217L6.03223 8.5H5.5C5.08579 8.5 4.75 8.16421 4.75 7.75C4.75 7.33579 5.08579 7 5.5 7H8.25V6.25C8.25 5.00736 9.25736 4 10.5 4H13.5ZM7.86621 16.1562C7.89013 16.7063 7.90624 17.0751 7.94141 17.3584C7.97545 17.6326 8.02151 17.7644 8.06934 17.8516C8.19271 18.0763 8.38239 18.2577 8.6123 18.3711C8.70153 18.4151 8.83504 18.4545 9.11035 18.4766C9.39482 18.4994 9.76335 18.5 10.3135 18.5H13.6865C14.2367 18.5 14.6052 18.4994 14.8896 18.4766C15.165 18.4545 15.2985 18.4151 15.3877 18.3711C15.6176 18.2577 15.8073 18.0763 15.9307 17.8516C15.9785 17.7644 16.0245 17.6326 16.0586 17.3584C16.0938 17.0751 16.1099 16.7063 16.1338 16.1562L16.4668 8.5H7.5332L7.86621 16.1562ZM9.97656 10.75C10.3906 10.7371 10.7371 11.0626 10.75 11.4766L10.875 15.4766C10.8879 15.8906 10.5624 16.2371 10.1484 16.25C9.73443 16.2629 9.38794 15.9374 9.375 15.5234L9.25 11.5234C9.23706 11.1094 9.56255 10.7629 9.97656 10.75ZM14.0244 10.75C14.4384 10.7635 14.7635 11.1105 14.75 11.5244L14.6201 15.5244C14.6066 15.9384 14.2596 16.2634 13.8457 16.25C13.4317 16.2365 13.1067 15.8896 13.1201 15.4756L13.251 11.4756C13.2645 11.0617 13.6105 10.7366 14.0244 10.75ZM10.5 5.5C10.0858 5.5 9.75 5.83579 9.75 6.25V7H14.25V6.25C14.25 5.83579 13.9142 5.5 13.5 5.5H10.5Z",fill:"currentColor"})});var b2=({size:e=16})=>on("svg",{width:e,height:e,viewBox:"0 0 24 24",fill:"none",children:[on("g",{clipPath:"url(#clip0_2_53)",children:[ve("path",{d:"M16.25 16.25L7.75 7.75",stroke:"currentColor",strokeWidth:"1.5",strokeLinecap:"round",strokeLinejoin:"round"}),ve("path",{d:"M7.75 16.25L16.25 7.75",stroke:"currentColor",strokeWidth:"1.5",strokeLinecap:"round",strokeLinejoin:"round"})]}),ve("defs",{children:ve("clipPath",{id:"clip0_2_53",children:ve("rect",{width:"24",height:"24",fill:"white"})})})]});var k2=({size:e=16})=>on("svg",{width:e,height:e,viewBox:"0 0 20 20",fill:"none",children:[ve("path",{d:"M9.99999 12.7082C11.4958 12.7082 12.7083 11.4956 12.7083 9.99984C12.7083 8.50407 11.4958 7.2915 9.99999 7.2915C8.50422 7.2915 7.29166 8.50407 7.29166 9.99984C7.29166 11.4956 8.50422 12.7082 9.99999 12.7082Z",stroke:"currentColor",strokeWidth:"1.25",strokeLinecap:"round",strokeLinejoin:"round"}),ve("path",{d:"M10 3.9585V5.05698",stroke:"currentColor",strokeWidth:"1.25",strokeLinecap:"round",strokeLinejoin:"round"}),ve("path",{d:"M10 14.9429V16.0414",stroke:"currentColor",strokeWidth:"1.25",strokeLinecap:"round",strokeLinejoin:"round"}),ve("path",{d:"M5.7269 5.72656L6.50682 6.50649",stroke:"currentColor",strokeWidth:"1.25",strokeLinecap:"round",strokeLinejoin:"round"}),ve("path",{d:"M13.4932 13.4932L14.2731 14.2731",stroke:"currentColor",strokeWidth:"1.25",strokeLinecap:"round",strokeLinejoin:"round"}),ve("path",{d:"M3.95834 10H5.05683",stroke:"currentColor",strokeWidth:"1.25",strokeLinecap:"round",strokeLinejoin:"round"}),ve("path",{d:"M14.9432 10H16.0417",stroke:"currentColor",strokeWidth:"1.25",strokeLinecap:"round",strokeLinejoin:"round"}),ve("path",{d:"M5.7269 14.2731L6.50682 13.4932",stroke:"currentColor",strokeWidth:"1.25",strokeLinecap:"round",strokeLinejoin:"round"}),ve("path",{d:"M13.4932 6.50649L14.2731 5.72656",stroke:"currentColor",strokeWidth:"1.25",strokeLinecap:"round",strokeLinejoin:"round"})]}),C2=({size:e=16})=>ve("svg",{width:e,height:e,viewBox:"0 0 20 20",fill:"none",children:ve("path",{d:"M15.5 10.4955C15.4037 11.5379 15.0124 12.5314 14.3721 13.3596C13.7317 14.1878 12.8688 14.8165 11.8841 15.1722C10.8995 15.5278 9.83397 15.5957 8.81217 15.3679C7.79038 15.1401 6.8546 14.6259 6.11434 13.8857C5.37408 13.1454 4.85995 12.2096 4.63211 11.1878C4.40427 10.166 4.47215 9.10048 4.82781 8.11585C5.18346 7.13123 5.81218 6.26825 6.64039 5.62791C7.4686 4.98756 8.46206 4.59634 9.5045 4.5C8.89418 5.32569 8.60049 6.34302 8.67685 7.36695C8.75321 8.39087 9.19454 9.35339 9.92058 10.0794C10.6466 10.8055 11.6091 11.2468 12.6331 11.3231C13.657 11.3995 14.6743 11.1058 15.5 10.4955Z",stroke:"currentColor",strokeWidth:"1.13793",strokeLinecap:"round",strokeLinejoin:"round"})}),S2=({size:e=16})=>ve("svg",{width:e,height:e,viewBox:"0 0 16 16",fill:"none",xmlns:"http://www.w3.org/2000/svg",children:ve("path",{d:"M11.3799 6.9572L9.05645 4.63375M11.3799 6.9572L6.74949 11.5699C6.61925 11.6996 6.45577 11.791 6.277 11.8339L4.29549 12.3092C3.93194 12.3964 3.60478 12.0683 3.69297 11.705L4.16585 9.75693C4.20893 9.57947 4.29978 9.4172 4.42854 9.28771L9.05645 4.63375M11.3799 6.9572L12.3455 5.98759C12.9839 5.34655 12.9839 4.31002 12.3455 3.66897C11.7033 3.02415 10.6594 3.02415 10.0172 3.66897L9.06126 4.62892L9.05645 4.63375",stroke:"currentColor",strokeWidth:"0.9",strokeLinecap:"round",strokeLinejoin:"round"})});var M2=({size:e=16})=>ve("svg",{width:e,height:e,viewBox:"0 0 16 16",fill:"none",xmlns:"http://www.w3.org/2000/svg",children:ve("path",{d:"M8.5 3.5L4 8L8.5 12.5",stroke:"currentColor",strokeWidth:"1.5",strokeLinecap:"round",strokeLinejoin:"round"})});var E2=({size:e=24})=>on("svg",{width:e,height:e,viewBox:"0 0 24 24",fill:"none",children:[ve("rect",{x:"3",y:"3",width:"18",height:"18",rx:"2",stroke:"currentColor",strokeWidth:"1.5"}),ve("line",{x1:"3",y1:"9",x2:"21",y2:"9",stroke:"currentColor",strokeWidth:"1.5"}),ve("line",{x1:"9",y1:"9",x2:"9",y2:"21",stroke:"currentColor",strokeWidth:"1.5"})]}),I2=({content:e,children:t,...n})=>{let[o,r]=(0,Io.useState)(!1),[i,l]=(0,Io.useState)(!1),[s,a]=(0,Io.useState)({top:0,right:0}),c=(0,Io.useRef)(null),f=(0,Io.useRef)(null),u=(0,Io.useRef)(null),x=()=>{if(c.current){let N=c.current.getBoundingClientRect();a({top:N.top+N.height/2,right:window.innerWidth-N.left+8})}},S=()=>{l(!0),u.current&&(clearTimeout(u.current),u.current=null),x(),f.current=nt(()=>{r(!0)},500)},b=()=>{f.current&&(clearTimeout(f.current),f.current=null),r(!1),u.current=nt(()=>{l(!1)},150)};return(0,Io.useEffect)(()=>()=>{f.current&&clearTimeout(f.current),u.current&&clearTimeout(u.current)},[]),N2(L2,{children:[P0("span",{ref:c,onMouseEnter:S,onMouseLeave:b,...n,children:t}),i&&(0,Ng.createPortal)(P0("div",{"data-feedback-toolbar":!0,style:{position:"fixed",top:s.top,right:s.right,transform:"translateY(-50%)",padding:"6px 10px",background:"#383838",color:"rgba(255, 255, 255, 0.7)",fontSize:"11px",fontWeight:400,lineHeight:"14px",borderRadius:"10px",width:"180px",textAlign:"left",zIndex:100020,pointerEvents:"none",boxShadow:"0px 1px 8px rgba(0, 0, 0, 0.28)",opacity:o?1:0,transition:"opacity 0.15s ease"},children:e}),document.body)]})},R2=`.styles-module__tooltip___mcXL2 {
  display: flex;
  justify-content: center;
  align-items: center;
  cursor: help;
}

.styles-module__tooltipIcon___Nq2nD {
  transform: translateY(0.5px);
  color: #fff;
  opacity: 0.2;
  transition: opacity 0.15s ease;
  will-change: transform;
}
.styles-module__tooltip___mcXL2:hover .styles-module__tooltipIcon___Nq2nD {
  opacity: 0.5;
}
[data-agentation-theme=light] .styles-module__tooltipIcon___Nq2nD {
  color: #000;
}`,D0={tooltip:"styles-module__tooltip___mcXL2",tooltipIcon:"styles-module__tooltipIcon___Nq2nD"},ii=({content:e})=>B0(I2,{className:D0.tooltip,content:e,children:B0(p2,{className:D0.tooltipIcon})}),$2=`.styles-module__toolbar___wNsdK svg[fill=none],
.styles-module__markersLayer___-25j1 svg[fill=none],
.styles-module__fixedMarkersLayer___ffyX6 svg[fill=none] {
  fill: none !important;
}
.styles-module__toolbar___wNsdK svg[fill=none] :not([fill]),
.styles-module__markersLayer___-25j1 svg[fill=none] :not([fill]),
.styles-module__fixedMarkersLayer___ffyX6 svg[fill=none] :not([fill]) {
  fill: none !important;
}

.styles-module__controlsContent___9GJWU :where(button, input, select, textarea, label) {
  background: unset;
  border: unset;
  border-radius: unset;
  padding: unset;
  margin: unset;
  color: unset;
  font-family: unset;
  font-weight: unset;
  font-style: unset;
  line-height: unset;
  letter-spacing: unset;
  text-transform: unset;
  text-decoration: unset;
  box-shadow: unset;
  outline: unset;
}

@keyframes styles-module__toolbarEnter___u8RRu {
  from {
    opacity: 0;
    transform: scale(0.5) rotate(90deg);
  }
  to {
    opacity: 1;
    transform: scale(1) rotate(0deg);
  }
}
@keyframes styles-module__toolbarHide___y8kaT {
  from {
    opacity: 1;
    transform: scale(1);
  }
  to {
    opacity: 0;
    transform: scale(0.8);
  }
}
@keyframes styles-module__badgeEnter___mVQLj {
  from {
    opacity: 0;
    transform: scale(0);
  }
  to {
    opacity: 1;
    transform: scale(1);
  }
}
@keyframes styles-module__scaleIn___c-r1K {
  from {
    opacity: 0;
    transform: scale(0.85);
  }
  to {
    opacity: 1;
    transform: scale(1);
  }
}
@keyframes styles-module__scaleOut___Wctwz {
  from {
    opacity: 1;
    transform: scale(1);
  }
  to {
    opacity: 0;
    transform: scale(0.85);
  }
}
@keyframes styles-module__slideUp___kgD36 {
  from {
    opacity: 0;
    transform: scale(0.85) translateY(8px);
  }
  to {
    opacity: 1;
    transform: scale(1) translateY(0);
  }
}
@keyframes styles-module__slideDown___zcdje {
  from {
    opacity: 1;
    transform: scale(1) translateY(0);
  }
  to {
    opacity: 0;
    transform: scale(0.85) translateY(8px);
  }
}
@keyframes styles-module__fadeIn___b9qmf {
  from {
    opacity: 0;
  }
  to {
    opacity: 1;
  }
}
@keyframes styles-module__fadeOut___6Ut6- {
  from {
    opacity: 1;
  }
  to {
    opacity: 0;
  }
}
@keyframes styles-module__hoverHighlightIn___6WYHY {
  from {
    opacity: 0;
    transform: scale(0.98);
  }
  to {
    opacity: 1;
    transform: scale(1);
  }
}
@keyframes styles-module__hoverTooltipIn___FYGQx {
  from {
    opacity: 0;
    transform: scale(0.95) translateY(4px);
  }
  to {
    opacity: 1;
    transform: scale(1) translateY(0);
  }
}
.styles-module__disableTransitions___EopxO :is(*, *::before, *::after) {
  transition: none !important;
}

:host {
  /* Set here rather than inline so a consumer className rule can still hide the toolbar. */
  display: contents;
  position: fixed;
  top: auto;
  left: auto;
  bottom: 1.25rem;
  right: 1.25rem;
  z-index: 100000;
}

.styles-module__positionContext___AZFHE,
.styles-module__toolbar___wNsdK {
  position: inherit;
  top: inherit;
  left: inherit;
  bottom: inherit;
  right: inherit;
  z-index: inherit;
}

.styles-module__toolbar___wNsdK {
  width: 337px;
  font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
  pointer-events: none;
  transition: left 0.36s cubic-bezier(0.19, 1, 0.22, 1), top 0s, right 0s, bottom 0s;
}
.styles-module__toolbar___wNsdK[data-dragging=true] {
  transition: none;
}

.styles-module__toolbarContainer___dIhma {
  position: relative;
  -webkit-user-select: none;
  user-select: none;
  margin-left: auto;
  align-self: flex-end;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #1a1a1a;
  color: #fff;
  border: none;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.2), 0 4px 16px rgba(0, 0, 0, 0.1);
  pointer-events: auto;
  transition: width 0.36s cubic-bezier(0.19, 1, 0.22, 1), transform 0.36s cubic-bezier(0.19, 1, 0.22, 1);
}
.styles-module__toolbarContainer___dIhma.styles-module__entrance___sgHd8 {
  animation: styles-module__toolbarEnter___u8RRu 0.5s cubic-bezier(0.34, 1.2, 0.64, 1) forwards;
}
.styles-module__toolbarContainer___dIhma.styles-module__hiding___1td44 {
  animation: styles-module__toolbarHide___y8kaT 0.4s cubic-bezier(0.4, 0, 1, 1) forwards;
  pointer-events: none;
}
.styles-module__toolbarContainer___dIhma.styles-module__collapsed___Rydsn {
  width: 44px;
  height: 44px;
  border-radius: 22px;
  padding: 0;
  cursor: pointer;
}
.styles-module__toolbarContainer___dIhma.styles-module__collapsed___Rydsn:hover {
  background: #2a2a2a;
}
.styles-module__toolbarContainer___dIhma.styles-module__collapsed___Rydsn:active {
  transform: scale(0.95);
}
.styles-module__toolbarContainer___dIhma.styles-module__expanded___ofKPx {
  height: 44px;
  border-radius: 22px;
  padding: 5px;
  width: 297px;
}
.styles-module__toolbarContainer___dIhma.styles-module__expanded___ofKPx.styles-module__serverConnected___Gfbou {
  width: 337px;
}

@media (prefers-reduced-motion: reduce) {
  .styles-module__toolbar___wNsdK,
  .styles-module__toolbarContainer___dIhma {
    transition: none;
  }
}
.styles-module__buttonWrapper___rBcdv.styles-module__toggleWrapper___7N0-q {
  position: absolute;
  top: 0;
  right: 0;
  width: 44px;
  height: 44px;
}

.styles-module__togglePlaceholder___wnqrL {
  width: 34px;
  flex: 0 0 34px;
  height: 34px;
}

.styles-module__toggleContent___0yfyP {
  position: absolute;
  top: 0;
  right: 0;
  width: 44px;
  height: 44px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: 50%;
  padding: 0;
  margin: 0;
  background: transparent;
  color: #fff;
  cursor: pointer;
  transition: color 0.15s ease;
}
.styles-module__toggleContent___0yfyP::before {
  content: "";
  position: absolute;
  inset: 5px;
  border-radius: 50%;
  pointer-events: none;
  background: transparent;
  transition: background-color 0.15s ease, transform 0.1s ease;
}
.styles-module__toggleContent___0yfyP.styles-module__expandedToggle___F7SRN {
  color: rgba(255, 255, 255, 0.85);
}
.styles-module__toggleContent___0yfyP.styles-module__expandedToggle___F7SRN:hover {
  color: #fff;
}
.styles-module__toggleContent___0yfyP.styles-module__expandedToggle___F7SRN:hover::before {
  background: rgba(255, 255, 255, 0.12);
}
.styles-module__toggleContent___0yfyP.styles-module__expandedToggle___F7SRN:active::before, .styles-module__toggleContent___0yfyP.styles-module__expandedToggle___F7SRN:active .styles-module__toggleGlyph___R7Oom {
  transform: scale(0.92);
}

.styles-module__toggleIcon___Jbtus {
  transform: translateY(-0.5px);
  transition: transform 0.3s cubic-bezier(0.22, 1, 0.36, 1);
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
}

.styles-module__expandedToggle___F7SRN .styles-module__toggleIcon___Jbtus {
  transform: none;
}

.styles-module__toggleGlyph___R7Oom {
  overflow: visible;
  transition: transform 0.1s ease;
}
.styles-module__toggleGlyph___R7Oom path {
  transform-box: fill-box;
  transform-origin: center;
  transition: transform 0.3s cubic-bezier(0.22, 1, 0.36, 1), opacity 0.16s ease;
}

.styles-module__toggleTopLine___hQaCm,
.styles-module__toggleMiddleLine___sFFVe {
  vector-effect: non-scaling-stroke;
}

.styles-module__toggleBottomLine___V-jX3 {
  transform-origin: left center;
}

.styles-module__toggleGlyph___R7Oom[data-active=true] .styles-module__toggleTopLine___hQaCm {
  transform: translateY(5.25px) rotate(45deg) scaleX(1.1422494);
}
.styles-module__toggleGlyph___R7Oom[data-active=true] .styles-module__toggleMiddleLine___sFFVe {
  transform: translateX(3.5px) rotate(-45deg) scaleX(2.4748737);
}
.styles-module__toggleGlyph___R7Oom[data-active=true] .styles-module__toggleBottomLine___V-jX3 {
  transform: scaleX(0);
  opacity: 0;
}
.styles-module__toggleGlyph___R7Oom[data-active=true] .styles-module__toggleSparkle___eeF99 {
  transform: scale(0);
  opacity: 0;
}

.styles-module__toggleContent___0yfyP:focus-visible,
.styles-module__controlButton___8Q0jc:focus-visible {
  outline: 2px solid var(--agentation-color-accent);
  outline-offset: 3px;
}

.styles-module__controlsContent___9GJWU {
  display: flex;
  align-items: center;
  gap: 6px;
  transition: filter 0.14s ease-out, opacity 0.14s ease-out, transform 0.36s cubic-bezier(0.19, 1, 0.22, 1);
}
.styles-module__controlsContent___9GJWU.styles-module__visible___KHwEW {
  transition: filter 0.3s cubic-bezier(0.22, 1, 0.36, 1), opacity 0.24s ease-out, transform 0.42s cubic-bezier(0.19, 1, 0.22, 1);
  opacity: 1;
  filter: blur(0px);
  transform: scale(1);
  visibility: visible;
  pointer-events: auto;
}
.styles-module__controlsContent___9GJWU.styles-module__hidden___Ae8H4 {
  pointer-events: none;
  opacity: 0;
  filter: blur(6px);
  transform: scale(0.4);
}

@media (prefers-reduced-motion: reduce) {
  .styles-module__controlsContent___9GJWU,
  .styles-module__controlsContent___9GJWU.styles-module__visible___KHwEW,
  .styles-module__toggleContent___0yfyP,
  .styles-module__toggleContent___0yfyP::before,
  .styles-module__toggleGlyph___R7Oom,
  .styles-module__toggleIcon___Jbtus,
  .styles-module__toggleGlyph___R7Oom path {
    transition: none;
  }
  .styles-module__controlsContent___9GJWU.styles-module__hidden___Ae8H4 {
    filter: none;
    transform: none;
  }
}
.styles-module__badge___2XsgF {
  position: absolute;
  top: -13px;
  right: -13px;
  -webkit-user-select: none;
  user-select: none;
  min-width: 18px;
  height: 18px;
  padding: 0 5px;
  border-radius: 9px;
  background-color: var(--agentation-color-accent);
  color: white;
  font-size: 0.625rem;
  font-weight: 600;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.15), inset 0 0 0 1px rgba(255, 255, 255, 0.04);
  opacity: 1;
  transition: transform 0.3s ease, opacity 0.2s ease;
  transform: scale(1);
}
.styles-module__badge___2XsgF.styles-module__fadeOut___6Ut6- {
  opacity: 0;
  transform: scale(0);
  pointer-events: none;
}
.styles-module__badge___2XsgF.styles-module__entrance___sgHd8 {
  animation: styles-module__badgeEnter___mVQLj 0.3s cubic-bezier(0.34, 1.2, 0.64, 1) 0.4s both;
}

.styles-module__controlButton___8Q0jc {
  position: relative;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  border-radius: 50%;
  border: none;
  background: transparent;
  color: rgba(255, 255, 255, 0.85);
  transition: background-color 0.15s ease, color 0.15s ease, transform 0.1s ease, opacity 0.2s ease;
}
.styles-module__controlButton___8Q0jc:hover:not(:disabled):not([data-active=true]):not([data-failed=true]):not([data-auto-sync=true]):not([data-error=true]):not([data-no-hover=true]) {
  background: rgba(255, 255, 255, 0.12);
  color: #fff;
}
.styles-module__controlButton___8Q0jc:active:not(:disabled) {
  transform: scale(0.92);
}
.styles-module__controlButton___8Q0jc:disabled {
  opacity: 0.35;
  cursor: not-allowed;
}
.styles-module__controlButton___8Q0jc[data-active=true] {
  color: var(--agentation-color-blue);
  background-color: color-mix(in srgb, var(--agentation-color-blue) 25%, transparent);
}
.styles-module__controlButton___8Q0jc[data-error=true] {
  color: var(--agentation-color-red);
  background-color: color-mix(in srgb, var(--agentation-color-red) 25%, transparent);
}
.styles-module__controlButton___8Q0jc[data-danger]:hover:not(:disabled):not([data-active=true]):not([data-failed=true]) {
  background-color: color-mix(in srgb, var(--agentation-color-red) 25%, transparent);
  color: var(--agentation-color-red);
}
.styles-module__controlButton___8Q0jc[data-no-hover=true], .styles-module__controlButton___8Q0jc.styles-module__statusShowing___te6iu {
  cursor: default;
  pointer-events: none;
  background: transparent !important;
}
.styles-module__controlButton___8Q0jc[data-auto-sync=true] {
  color: var(--agentation-color-green);
  background: transparent;
  cursor: default;
}
.styles-module__controlButton___8Q0jc[data-failed=true] {
  color: var(--agentation-color-red);
  background-color: color-mix(in srgb, var(--agentation-color-red) 25%, transparent);
}

.styles-module__buttonBadge___NeFWb {
  position: absolute;
  top: 0px;
  right: 0px;
  min-width: 16px;
  height: 16px;
  padding: 0 4px;
  border-radius: 8px;
  background-color: var(--agentation-color-accent);
  color: white;
  font-size: 0.625rem;
  font-weight: 600;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 0 0 2px #1a1a1a, 0 1px 3px rgba(0, 0, 0, 0.2);
  pointer-events: none;
}
[data-agentation-theme=light] .styles-module__buttonBadge___NeFWb {
  box-shadow: 0 0 0 2px #fff, 0 1px 3px rgba(0, 0, 0, 0.2);
}

@keyframes styles-module__mcpIndicatorPulseConnected___EDodZ {
  0%, 100% {
    box-shadow: 0 0 0 0 color-mix(in srgb, var(--agentation-color-green) 50%, transparent);
  }
  50% {
    box-shadow: 0 0 0 5px color-mix(in srgb, var(--agentation-color-green) 0%, transparent);
  }
}
@keyframes styles-module__mcpIndicatorPulseConnecting___cCYte {
  0%, 100% {
    box-shadow: 0 0 0 0 color-mix(in srgb, var(--agentation-color-yellow) 50%, transparent);
  }
  50% {
    box-shadow: 0 0 0 5px color-mix(in srgb, var(--agentation-color-yellow) 0%, transparent);
  }
}
.styles-module__mcpIndicator___zGJeL {
  position: absolute;
  top: 3px;
  right: 3px;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  pointer-events: none;
  transition: background-color 0.3s ease, opacity 0.15s ease, transform 0.15s ease;
  opacity: 1;
  transform: scale(1);
}
.styles-module__mcpIndicator___zGJeL.styles-module__connected___7c28g {
  background-color: var(--agentation-color-green);
  animation: styles-module__mcpIndicatorPulseConnected___EDodZ 2.5s ease-in-out infinite;
}
.styles-module__mcpIndicator___zGJeL.styles-module__connecting___uo-CW {
  background-color: var(--agentation-color-yellow);
  animation: styles-module__mcpIndicatorPulseConnecting___cCYte 1.5s ease-in-out infinite;
}
.styles-module__mcpIndicator___zGJeL.styles-module__hidden___Ae8H4 {
  opacity: 0;
  transform: scale(0);
  animation: none;
}

@keyframes styles-module__connectionPulse___-Zycw {
  0%, 100% {
    opacity: 1;
    transform: scale(1);
  }
  50% {
    opacity: 0.6;
    transform: scale(0.9);
  }
}
.styles-module__connectionIndicatorWrapper___L-e-3 {
  width: 8px;
  height: 34px;
  margin-left: 6px;
  margin-right: 6px;
}

.styles-module__connectionIndicator___afk9p {
  position: relative;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  opacity: 0;
  transition: opacity 0.3s ease, background-color 0.3s ease;
  cursor: default;
}

.styles-module__connectionIndicatorVisible___C-i5B {
  opacity: 1;
}

.styles-module__connectionIndicatorConnected___IY8pR {
  background-color: var(--agentation-color-green);
  animation: styles-module__connectionPulse___-Zycw 2.5s ease-in-out infinite;
}

.styles-module__connectionIndicatorDisconnected___kmpaZ {
  background-color: var(--agentation-color-red);
  animation: none;
}

.styles-module__connectionIndicatorConnecting___QmSLH {
  background-color: var(--agentation-color-yellow);
  animation: styles-module__connectionPulse___-Zycw 1s ease-in-out infinite;
}

.styles-module__buttonWrapper___rBcdv {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
}
.styles-module__buttonWrapper___rBcdv:hover .styles-module__buttonTooltip___Burd9 {
  opacity: 1;
  visibility: visible;
  transform: translateX(-50%) scale(1);
  transition-delay: 0.85s;
}
.styles-module__buttonWrapper___rBcdv:has(.styles-module__controlButton___8Q0jc:disabled):hover .styles-module__buttonTooltip___Burd9 {
  opacity: 0;
  visibility: hidden;
}

.styles-module__tooltipsInSession___-0lHH .styles-module__buttonWrapper___rBcdv:hover .styles-module__buttonTooltip___Burd9 {
  transition-delay: 0s;
}

.styles-module__sendButtonWrapper___UUxG6 {
  width: 0;
  opacity: 0;
  overflow: hidden;
  pointer-events: none;
  margin-left: -6px;
  transition: width 0.36s cubic-bezier(0.19, 1, 0.22, 1), opacity 0.2s cubic-bezier(0.19, 1, 0.22, 1), margin 0.36s cubic-bezier(0.19, 1, 0.22, 1);
}
.styles-module__sendButtonWrapper___UUxG6 .styles-module__controlButton___8Q0jc {
  transform: scale(0.8);
  transition: transform 0.36s cubic-bezier(0.19, 1, 0.22, 1);
}
.styles-module__sendButtonWrapper___UUxG6.styles-module__sendButtonVisible___WPSQU {
  width: 34px;
  opacity: 1;
  overflow: visible;
  pointer-events: auto;
  margin-left: 0;
}
.styles-module__sendButtonWrapper___UUxG6.styles-module__sendButtonVisible___WPSQU .styles-module__controlButton___8Q0jc {
  transform: scale(1);
}

.styles-module__buttonTooltip___Burd9 {
  position: absolute;
  bottom: calc(100% + 14px);
  left: 50%;
  transform: translateX(-50%) scale(0.95);
  padding: 6px 10px;
  background: #1a1a1a;
  color: rgba(255, 255, 255, 0.9);
  font-size: 12px;
  font-weight: 500;
  border-radius: 8px;
  white-space: nowrap;
  opacity: 0;
  visibility: hidden;
  pointer-events: none;
  z-index: 100001;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.3);
  transition: opacity 0.135s ease, transform 0.135s ease, visibility 0.135s ease;
}
.styles-module__buttonTooltip___Burd9::after {
  content: "";
  position: absolute;
  top: calc(100% - 4px);
  left: 50%;
  transform: translateX(-50%) rotate(45deg);
  width: 8px;
  height: 8px;
  background: #1a1a1a;
  border-radius: 0 0 2px 0;
}

.styles-module__shortcut___lEAQk {
  margin-left: 4px;
  opacity: 0.5;
}

.styles-module__tooltipBelow___m6ats .styles-module__buttonTooltip___Burd9 {
  bottom: auto;
  top: calc(100% + 14px);
  transform: translateX(-50%) scale(0.95);
}
.styles-module__tooltipBelow___m6ats .styles-module__buttonTooltip___Burd9::after {
  top: -4px;
  bottom: auto;
  border-radius: 2px 0 0 0;
}

.styles-module__tooltipBelow___m6ats .styles-module__buttonWrapper___rBcdv:hover .styles-module__buttonTooltip___Burd9 {
  transform: translateX(-50%) scale(1);
}

.styles-module__tooltipsHidden___VtLJG .styles-module__buttonTooltip___Burd9 {
  opacity: 0 !important;
  visibility: hidden !important;
  transition: none !important;
}

.styles-module__tooltipVisible___0jcCv,
.styles-module__tooltipsHidden___VtLJG .styles-module__tooltipVisible___0jcCv {
  opacity: 1 !important;
  visibility: visible !important;
  transform: translateX(-50%) scale(1) !important;
  transition-delay: 0s !important;
}

.styles-module__buttonWrapperAlignLeft___myzIp .styles-module__buttonTooltip___Burd9 {
  left: 50%;
  transform: translateX(-12px) scale(0.95);
}
.styles-module__buttonWrapperAlignLeft___myzIp .styles-module__buttonTooltip___Burd9::after {
  left: 16px;
}
.styles-module__buttonWrapperAlignLeft___myzIp:hover .styles-module__buttonTooltip___Burd9 {
  transform: translateX(-12px) scale(1);
}

.styles-module__tooltipBelow___m6ats .styles-module__buttonWrapperAlignLeft___myzIp .styles-module__buttonTooltip___Burd9 {
  transform: translateX(-12px) scale(0.95);
}
.styles-module__tooltipBelow___m6ats .styles-module__buttonWrapperAlignLeft___myzIp:hover .styles-module__buttonTooltip___Burd9 {
  transform: translateX(-12px) scale(1);
}

.styles-module__buttonWrapperAlignRight___HCQFR .styles-module__buttonTooltip___Burd9 {
  left: 50%;
  transform: translateX(calc(-100% + 12px)) scale(0.95);
}
.styles-module__buttonWrapperAlignRight___HCQFR .styles-module__buttonTooltip___Burd9::after {
  left: auto;
  right: 8px;
}
.styles-module__buttonWrapperAlignRight___HCQFR:hover .styles-module__buttonTooltip___Burd9 {
  transform: translateX(calc(-100% + 12px)) scale(1);
}

.styles-module__tooltipBelow___m6ats .styles-module__buttonWrapperAlignRight___HCQFR .styles-module__buttonTooltip___Burd9 {
  transform: translateX(calc(-100% + 12px)) scale(0.95);
}
.styles-module__tooltipBelow___m6ats .styles-module__buttonWrapperAlignRight___HCQFR:hover .styles-module__buttonTooltip___Burd9 {
  transform: translateX(calc(-100% + 12px)) scale(1);
}

.styles-module__divider___c--s1 {
  width: 1px;
  height: 12px;
  background: rgba(255, 255, 255, 0.15);
  margin: 0 3px;
}

.styles-module__overlay___Q1O9y {
  position: fixed;
  inset: 0;
  z-index: 99997;
  pointer-events: none;
}
.styles-module__overlay___Q1O9y > * {
  pointer-events: auto;
}

.styles-module__hoverHighlight___ogakW {
  position: fixed;
  border: 2px solid color-mix(in srgb, var(--agentation-color-accent) 50%, transparent);
  border-radius: 4px;
  background-color: color-mix(in srgb, var(--agentation-color-accent) 4%, transparent);
  pointer-events: none !important;
  box-sizing: border-box;
  will-change: opacity;
  contain: layout style;
}
.styles-module__hoverHighlight___ogakW.styles-module__enter___WFIki {
  animation: styles-module__hoverHighlightIn___6WYHY 0.12s ease-out forwards;
}

.styles-module__multiSelectOutline___cSJ-m {
  position: fixed;
  border: 2px dashed color-mix(in srgb, var(--agentation-color-green) 60%, transparent);
  border-radius: 4px;
  pointer-events: none !important;
  background-color: color-mix(in srgb, var(--agentation-color-green) 5%, transparent);
  box-sizing: border-box;
  will-change: opacity;
}
.styles-module__multiSelectOutline___cSJ-m.styles-module__enter___WFIki {
  animation: styles-module__fadeIn___b9qmf 0.15s ease-out forwards;
}
.styles-module__multiSelectOutline___cSJ-m.styles-module__exit___fyOJ0 {
  animation: styles-module__fadeOut___6Ut6- 0.15s ease-out forwards;
}

.styles-module__singleSelectOutline___QhX-O {
  position: fixed;
  border: 2px solid color-mix(in srgb, var(--agentation-color-blue) 60%, transparent);
  border-radius: 4px;
  pointer-events: none !important;
  background-color: color-mix(in srgb, var(--agentation-color-blue) 5%, transparent);
  box-sizing: border-box;
  will-change: opacity;
}
.styles-module__singleSelectOutline___QhX-O.styles-module__enter___WFIki {
  animation: styles-module__fadeIn___b9qmf 0.15s ease-out forwards;
}
.styles-module__singleSelectOutline___QhX-O.styles-module__exit___fyOJ0 {
  animation: styles-module__fadeOut___6Ut6- 0.15s ease-out forwards;
}

.styles-module__hoverTooltip___bvLk7 {
  position: fixed;
  z-index: 99999;
  font-size: 0.6875rem;
  font-weight: 500;
  color: #fff;
  background: rgba(0, 0, 0, 0.85);
  padding: 0.35rem 0.6rem;
  border-radius: 0.375rem;
  pointer-events: none !important;
  white-space: nowrap;
  max-width: min(280px, 100vw - 16px - 1.2rem);
  overflow: hidden;
  text-overflow: ellipsis;
}
.styles-module__hoverTooltip___bvLk7.styles-module__enter___WFIki {
  animation: styles-module__hoverTooltipIn___FYGQx 0.1s ease-out forwards;
}

.styles-module__hoverReactPath___gx1IJ {
  font-size: 0.625rem;
  color: rgba(255, 255, 255, 0.6);
  margin-bottom: 0.15rem;
  overflow: hidden;
  text-overflow: ellipsis;
}

.styles-module__hoverElementName___QMLMl {
  overflow: hidden;
  text-overflow: ellipsis;
}

.styles-module__markersLayer___-25j1 {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  height: 0;
  z-index: 99998;
  pointer-events: none;
}
.styles-module__markersLayer___-25j1 > * {
  pointer-events: auto;
}

.styles-module__fixedMarkersLayer___ffyX6 {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  z-index: 99998;
  pointer-events: none;
}
.styles-module__fixedMarkersLayer___ffyX6 > * {
  pointer-events: auto;
}

.styles-module__marker___6sQrs {
  position: absolute;
  width: 22px;
  height: 22px;
  background: var(--agentation-color-blue);
  color: white;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 0.6875rem;
  font-weight: 600;
  transform: translate(-50%, -50%) scale(1);
  opacity: 1;
  cursor: pointer;
  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.2), inset 0 0 0 1px rgba(0, 0, 0, 0.04);
  -webkit-user-select: none;
  user-select: none;
  will-change: transform, opacity;
  contain: layout style;
  z-index: 1;
}
.styles-module__marker___6sQrs:hover {
  z-index: 2;
}
.styles-module__marker___6sQrs:not(.styles-module__enter___WFIki):not(.styles-module__exit___fyOJ0):not(.styles-module__clearing___FQ--7) {
  transition: background-color 0.15s ease, transform 0.1s ease;
}
.styles-module__marker___6sQrs.styles-module__enter___WFIki {
  animation: styles-module__markerIn___5FaAP 0.25s cubic-bezier(0.22, 1, 0.36, 1) both;
}
.styles-module__marker___6sQrs.styles-module__exit___fyOJ0 {
  animation: styles-module__markerOut___GU5jX 0.2s ease-out both;
  pointer-events: none;
}
.styles-module__marker___6sQrs.styles-module__clearing___FQ--7 {
  animation: styles-module__markerOut___GU5jX 0.15s ease-out both;
  pointer-events: none;
}
.styles-module__marker___6sQrs:not(.styles-module__enter___WFIki):not(.styles-module__exit___fyOJ0):not(.styles-module__clearing___FQ--7):hover {
  transform: translate(-50%, -50%) scale(1.1);
}
.styles-module__marker___6sQrs.styles-module__pending___2IHLC {
  position: fixed;
  background-color: var(--agentation-color-blue);
  cursor: default;
}
.styles-module__marker___6sQrs.styles-module__fixed___dBMHC {
  position: fixed;
}
.styles-module__marker___6sQrs.styles-module__multiSelect___YWiuz {
  background-color: var(--agentation-color-green);
  width: 26px;
  height: 26px;
  border-radius: 6px;
  font-size: 0.75rem;
}
.styles-module__marker___6sQrs.styles-module__multiSelect___YWiuz.styles-module__pending___2IHLC {
  background-color: var(--agentation-color-green);
}
.styles-module__marker___6sQrs.styles-module__hovered___ZgXIy {
  background-color: var(--agentation-color-red);
}

.styles-module__renumber___nCTxD {
  display: block;
  animation: styles-module__renumberRoll___Wgbq3 0.2s ease-out;
}

@keyframes styles-module__renumberRoll___Wgbq3 {
  0% {
    transform: translateX(-40%);
    opacity: 0;
  }
  100% {
    transform: translateX(0);
    opacity: 1;
  }
}
.styles-module__markerTooltip___aLJID {
  position: absolute;
  top: calc(100% + 10px);
  left: 50%;
  transform: translateX(-50%) scale(0.909);
  z-index: 100002;
  background: #1a1a1a;
  padding: 8px 0.75rem;
  border-radius: 0.75rem;
  font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
  font-weight: 400;
  color: #fff;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.3), 0 0 0 1px rgba(255, 255, 255, 0.08);
  min-width: 120px;
  max-width: 200px;
  pointer-events: none;
  cursor: default;
}
.styles-module__markerTooltip___aLJID.styles-module__enter___WFIki {
  animation: styles-module__tooltipIn___0N31w 0.1s ease-out forwards;
}

.styles-module__markerQuote___FHmrz {
  display: block;
  font-size: 12px;
  font-style: italic;
  color: rgba(255, 255, 255, 0.6);
  margin-bottom: 0.3125rem;
  line-height: 1.4;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.styles-module__markerNote___QkrrS {
  display: block;
  font-size: 13px;
  font-weight: 400;
  line-height: 1.4;
  color: #fff;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  padding-bottom: 2px;
}

.styles-module__markerHint___2iF-6 {
  display: block;
  font-size: 0.625rem;
  font-weight: 400;
  color: rgba(255, 255, 255, 0.6);
  margin-top: 0.375rem;
  white-space: nowrap;
}

.styles-module__settingsPanel___OxX3Y {
  position: absolute;
  right: 5px;
  bottom: calc(100% + 0.5rem);
  z-index: 1;
  overflow: hidden;
  background: #1c1c1c;
  border-radius: 1rem;
  padding: 13px 0 16px;
  min-width: 205px;
  cursor: default;
  opacity: 1;
  box-shadow: 0 1px 8px rgba(0, 0, 0, 0.25), 0 0 0 1px rgba(0, 0, 0, 0.04);
  transition: background-color 0.25s ease, box-shadow 0.25s ease;
}
.styles-module__settingsPanel___OxX3Y::before, .styles-module__settingsPanel___OxX3Y::after {
  content: "";
  position: absolute;
  top: 0;
  bottom: 0;
  width: 16px;
  z-index: 2;
  pointer-events: none;
}
.styles-module__settingsPanel___OxX3Y::before {
  left: 0;
  background: linear-gradient(to right, #1c1c1c 0%, transparent 100%);
}
.styles-module__settingsPanel___OxX3Y::after {
  right: 0;
  background: linear-gradient(to left, #1c1c1c 0%, transparent 100%);
}
.styles-module__settingsPanel___OxX3Y .styles-module__settingsHeader___pwDY9,
.styles-module__settingsPanel___OxX3Y .styles-module__settingsBrand___0gJeM,
.styles-module__settingsPanel___OxX3Y .styles-module__settingsBrandSlash___uTG18,
.styles-module__settingsPanel___OxX3Y .styles-module__settingsVersion___TUcFq,
.styles-module__settingsPanel___OxX3Y .styles-module__settingsSection___m-YM2,
.styles-module__settingsPanel___OxX3Y .styles-module__settingsLabel___8UjfX,
.styles-module__settingsPanel___OxX3Y .styles-module__cycleButton___FMKfw,
.styles-module__settingsPanel___OxX3Y .styles-module__cycleDot___nPgLY,
.styles-module__settingsPanel___OxX3Y .styles-module__dropdownButton___16NPz,
.styles-module__settingsPanel___OxX3Y .styles-module__toggleLabel___Xm8Aa,
.styles-module__settingsPanel___OxX3Y .styles-module__customCheckbox___U39ax,
.styles-module__settingsPanel___OxX3Y .styles-module__sliderLabel___U8sPr,
.styles-module__settingsPanel___OxX3Y .styles-module__slider___GLdxp,
.styles-module__settingsPanel___OxX3Y .styles-module__themeToggle___2rUjA {
  transition: background-color 0.25s ease, color 0.25s ease, border-color 0.25s ease;
}
.styles-module__settingsPanel___OxX3Y.styles-module__enter___WFIki {
  opacity: 1;
  transform: translateY(0) scale(1);
  filter: blur(0px);
  transition: opacity 0.2s ease, transform 0.2s ease, filter 0.2s ease;
}
.styles-module__settingsPanel___OxX3Y.styles-module__exit___fyOJ0 {
  opacity: 0;
  transform: translateY(8px) scale(0.95);
  filter: blur(5px);
  pointer-events: none;
  transition: opacity 0.1s ease, transform 0.1s ease, filter 0.1s ease;
}
[data-agentation-theme=dark] .styles-module__settingsPanel___OxX3Y {
  background: #1a1a1a;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.3), 0 0 0 1px rgba(255, 255, 255, 0.08);
}
[data-agentation-theme=dark] .styles-module__settingsPanel___OxX3Y .styles-module__settingsLabel___8UjfX {
  color: rgba(255, 255, 255, 0.6);
}
[data-agentation-theme=dark] .styles-module__settingsPanel___OxX3Y .styles-module__settingsOption___UNa12 {
  color: rgba(255, 255, 255, 0.85);
}
[data-agentation-theme=dark] .styles-module__settingsPanel___OxX3Y .styles-module__settingsOption___UNa12:hover {
  background: rgba(255, 255, 255, 0.1);
}
[data-agentation-theme=dark] .styles-module__settingsPanel___OxX3Y .styles-module__settingsOption___UNa12.styles-module__selected___OwRqP {
  background: rgba(255, 255, 255, 0.15);
  color: #fff;
}
[data-agentation-theme=dark] .styles-module__settingsPanel___OxX3Y .styles-module__toggleLabel___Xm8Aa {
  color: rgba(255, 255, 255, 0.85);
}

.styles-module__settingsPanelContainer___Xksv8 {
  overflow: visible;
  position: relative;
  display: flex;
  padding: 0 1rem;
}

.styles-module__settingsPage___6YfHH {
  min-width: 100%;
  flex-shrink: 0;
  transition: transform 0.2s ease, opacity 0.2s ease;
  transition-delay: 0s;
  opacity: 1;
}

.styles-module__settingsPage___6YfHH.styles-module__slideLeft___Ps01J {
  transform: translateX(-24px);
  opacity: 0;
  pointer-events: none;
}

.styles-module__automationsPage___uvCq6 {
  position: absolute;
  top: 0;
  left: 24px;
  width: 100%;
  height: 100%;
  padding: 3px 1rem 0;
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  transition: transform 0.2s ease, opacity 0.2s ease;
  opacity: 0;
  pointer-events: none;
}

.styles-module__automationsPage___uvCq6.styles-module__slideIn___4-qXe {
  transform: translateX(-24px);
  opacity: 1;
  pointer-events: auto;
}

.styles-module__settingsNavLink___wCzJt {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  padding: 0;
  border: none;
  background: transparent;
  font-family: inherit;
  font-size: 0.8125rem;
  font-weight: 400;
  color: rgba(255, 255, 255, 0.5);
  cursor: pointer;
  transition: color 0.15s ease;
}
.styles-module__settingsNavLink___wCzJt:hover {
  color: rgba(255, 255, 255, 0.9);
}
[data-agentation-theme=light] .styles-module__settingsNavLink___wCzJt {
  color: rgba(0, 0, 0, 0.5);
}
[data-agentation-theme=light] .styles-module__settingsNavLink___wCzJt:hover {
  color: rgba(0, 0, 0, 0.8);
}
.styles-module__settingsNavLink___wCzJt svg {
  color: rgba(255, 255, 255, 0.4);
  transition: color 0.15s ease;
}
.styles-module__settingsNavLink___wCzJt:hover svg {
  color: #fff;
}
[data-agentation-theme=light] .styles-module__settingsNavLink___wCzJt svg {
  color: rgba(0, 0, 0, 0.25);
}
[data-agentation-theme=light] .styles-module__settingsNavLink___wCzJt:hover svg {
  color: rgba(0, 0, 0, 0.8);
}

.styles-module__settingsNavLinkRight___ZWwhj {
  display: flex;
  align-items: center;
  gap: 6px;
}

.styles-module__mcpNavIndicator___cl9pO {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}
.styles-module__mcpNavIndicator___cl9pO.styles-module__connected___7c28g {
  background-color: var(--agentation-color-green);
  animation: styles-module__mcpPulse___uNggr 2.5s ease-in-out infinite;
}
.styles-module__mcpNavIndicator___cl9pO.styles-module__connecting___uo-CW {
  background-color: var(--agentation-color-yellow);
  animation: styles-module__mcpPulse___uNggr 1.5s ease-in-out infinite;
}

.styles-module__settingsBackButton___bIe2j {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 6px 0 12px 0;
  margin: -6px 0 0.5rem 0;
  border: none;
  border-bottom: 1px solid rgba(255, 255, 255, 0.07);
  border-radius: 0;
  background: transparent;
  font-family: inherit;
  font-size: 0.8125rem;
  font-weight: 500;
  letter-spacing: -0.15px;
  color: #fff;
  cursor: pointer;
  transition: transform 0.12s cubic-bezier(0.32, 0.72, 0, 1);
}
.styles-module__settingsBackButton___bIe2j svg {
  opacity: 0.4;
  flex-shrink: 0;
  transition: opacity 0.15s ease, transform 0.18s cubic-bezier(0.32, 0.72, 0, 1);
}
.styles-module__settingsBackButton___bIe2j:hover {
  border-bottom-color: rgba(255, 255, 255, 0.07);
}
.styles-module__settingsBackButton___bIe2j:hover svg {
  opacity: 1;
}
[data-agentation-theme=light] .styles-module__settingsBackButton___bIe2j {
  color: rgba(0, 0, 0, 0.85);
  border-bottom-color: rgba(0, 0, 0, 0.08);
}
[data-agentation-theme=light] .styles-module__settingsBackButton___bIe2j:hover {
  border-bottom-color: rgba(0, 0, 0, 0.08);
}

.styles-module__automationHeader___InP0r {
  display: flex;
  align-items: center;
  gap: 0.125rem;
  font-size: 0.8125rem;
  font-weight: 400;
  color: #fff;
}
[data-agentation-theme=light] .styles-module__automationHeader___InP0r {
  color: rgba(0, 0, 0, 0.85);
}

.styles-module__automationDescription___NKlmo {
  font-size: 0.6875rem;
  font-weight: 300;
  color: rgba(255, 255, 255, 0.5);
  margin-top: 2px;
  line-height: 14px;
}
[data-agentation-theme=light] .styles-module__automationDescription___NKlmo {
  color: rgba(0, 0, 0, 0.5);
}

.styles-module__learnMoreLink___8xv-x {
  color: rgba(255, 255, 255, 0.8);
  text-decoration: underline dotted;
  text-decoration-color: rgba(255, 255, 255, 0.2);
  text-underline-offset: 2px;
  transition: color 0.15s ease;
}
.styles-module__learnMoreLink___8xv-x:hover {
  color: #fff;
}
[data-agentation-theme=light] .styles-module__learnMoreLink___8xv-x {
  color: rgba(0, 0, 0, 0.6);
  text-decoration-color: rgba(0, 0, 0, 0.2);
}
[data-agentation-theme=light] .styles-module__learnMoreLink___8xv-x:hover {
  color: rgba(0, 0, 0, 0.85);
}

.styles-module__autoSendRow___UblX5 {
  display: flex;
  align-items: center;
  gap: 8px;
}

.styles-module__autoSendLabel___icDc2 {
  font-size: 0.6875rem;
  font-weight: 400;
  color: rgba(255, 255, 255, 0.4);
  transition: color 0.15s ease;
}
.styles-module__autoSendLabel___icDc2.styles-module__active___-zoN6 {
  color: #66b8ff;
  color: color(display-p3 0.4 0.72 1);
}
[data-agentation-theme=light] .styles-module__autoSendLabel___icDc2 {
  color: rgba(0, 0, 0, 0.4);
}
[data-agentation-theme=light] .styles-module__autoSendLabel___icDc2.styles-module__active___-zoN6 {
  color: var(--agentation-color-blue);
}

.styles-module__webhookUrlInput___2375C {
  display: block;
  width: 100%;
  flex: 1;
  min-height: 60px;
  box-sizing: border-box;
  margin-top: 11px;
  padding: 8px 10px;
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 6px;
  background: rgba(255, 255, 255, 0.03);
  font-family: inherit;
  font-size: 0.75rem;
  font-weight: 400;
  color: #fff;
  outline: none;
  resize: none;
  user-select: text;
  transition: border-color 0.15s ease, background-color 0.15s ease, box-shadow 0.15s ease;
}
.styles-module__webhookUrlInput___2375C::placeholder {
  color: rgba(255, 255, 255, 0.3);
}
.styles-module__webhookUrlInput___2375C:focus {
  border-color: rgba(255, 255, 255, 0.3);
  background: rgba(255, 255, 255, 0.08);
}
[data-agentation-theme=light] .styles-module__webhookUrlInput___2375C {
  border-color: rgba(0, 0, 0, 0.1);
  background: rgba(0, 0, 0, 0.03);
  color: rgba(0, 0, 0, 0.85);
}
[data-agentation-theme=light] .styles-module__webhookUrlInput___2375C::placeholder {
  color: rgba(0, 0, 0, 0.3);
}
[data-agentation-theme=light] .styles-module__webhookUrlInput___2375C:focus {
  border-color: rgba(0, 0, 0, 0.25);
  background: rgba(0, 0, 0, 0.05);
}

.styles-module__settingsHeader___pwDY9 {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 24px;
  margin-bottom: 0.5rem;
  padding-bottom: 9px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.07);
}

.styles-module__settingsBrand___0gJeM {
  font-size: 0.8125rem;
  font-weight: 600;
  letter-spacing: -0.0094em;
  color: #fff;
  text-decoration: none;
}

.styles-module__settingsBrandSlash___uTG18 {
  color: var(--agentation-color-accent);
  transition: color 0.2s ease;
}

.styles-module__settingsVersion___TUcFq {
  font-size: 11px;
  font-weight: 400;
  color: rgba(255, 255, 255, 0.4);
  margin-left: auto;
  letter-spacing: -0.0094em;
}

.styles-module__settingsSection___m-YM2 + .styles-module__settingsSection___m-YM2 {
  margin-top: 0.5rem;
  padding-top: 0.5rem;
  border-top: 1px solid rgba(255, 255, 255, 0.07);
}
.styles-module__settingsSection___m-YM2.styles-module__settingsSectionExtraPadding___jdhFV {
  padding-top: calc(0.5rem + 4px);
}

.styles-module__settingsSectionGrow___h-5HZ {
  flex: 1;
  display: flex;
  flex-direction: column;
}

.styles-module__settingsRow___3sdhc {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 24px;
}
.styles-module__settingsRow___3sdhc.styles-module__settingsRowMarginTop___zA0Sp {
  margin-top: 8px;
}

.styles-module__dropdownContainer___BVnxe {
  position: relative;
}

.styles-module__dropdownButton___16NPz {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.25rem 0.5rem;
  border: none;
  border-radius: 0.375rem;
  background: transparent;
  font-size: 0.8125rem;
  font-weight: 600;
  color: #fff;
  cursor: pointer;
  transition: background-color 0.15s ease, color 0.15s ease;
  letter-spacing: -0.0094em;
}
.styles-module__dropdownButton___16NPz:hover {
  background: rgba(255, 255, 255, 0.08);
}
.styles-module__dropdownButton___16NPz svg {
  opacity: 0.6;
}

.styles-module__cycleButton___FMKfw {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0;
  border: none;
  background: transparent;
  font-size: 0.8125rem;
  font-weight: 500;
  color: #fff;
  cursor: pointer;
  letter-spacing: -0.0094em;
}
[data-agentation-theme=light] .styles-module__cycleButton___FMKfw {
  color: rgba(0, 0, 0, 0.85);
}
.styles-module__cycleButton___FMKfw:disabled {
  opacity: 0.35;
  cursor: not-allowed;
}

.styles-module__settingsRowDisabled___EgS0V .styles-module__settingsLabel___8UjfX {
  color: rgba(255, 255, 255, 0.2);
}
[data-agentation-theme=light] .styles-module__settingsRowDisabled___EgS0V .styles-module__settingsLabel___8UjfX {
  color: rgba(0, 0, 0, 0.2);
}
.styles-module__settingsRowDisabled___EgS0V .styles-module__toggleSwitch___l4Ygm {
  opacity: 0.4;
  cursor: not-allowed;
}

@keyframes styles-module__cycleTextIn___Q6zJf {
  0% {
    opacity: 0;
    transform: translateY(-6px);
  }
  100% {
    opacity: 1;
    transform: translateY(0);
  }
}
.styles-module__cycleButtonText___fD1LR {
  display: inline-block;
  animation: styles-module__cycleTextIn___Q6zJf 0.2s ease-out;
}

.styles-module__cycleDots___LWuoQ {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.styles-module__cycleDot___nPgLY {
  width: 3px;
  height: 3px;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.3);
  transform: scale(0.667);
  transition: background-color 0.25s ease-out, transform 0.25s ease-out;
}
.styles-module__cycleDot___nPgLY.styles-module__active___-zoN6 {
  background: #fff;
  transform: scale(1);
}
[data-agentation-theme=light] .styles-module__cycleDot___nPgLY {
  background: rgba(0, 0, 0, 0.2);
}
[data-agentation-theme=light] .styles-module__cycleDot___nPgLY.styles-module__active___-zoN6 {
  background: rgba(0, 0, 0, 0.7);
}

.styles-module__dropdownMenu___k73ER {
  position: absolute;
  right: 0;
  top: calc(100% + 0.25rem);
  background: #1a1a1a;
  border-radius: 0.5rem;
  padding: 0.25rem;
  min-width: 120px;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.3), 0 0 0 1px rgba(255, 255, 255, 0.1);
  z-index: 10;
  animation: styles-module__scaleIn___c-r1K 0.15s ease-out;
}

.styles-module__dropdownItem___ylsLj {
  width: 100%;
  display: flex;
  align-items: center;
  padding: 0.5rem 0.625rem;
  border: none;
  border-radius: 0.375rem;
  background: transparent;
  font-size: 0.8125rem;
  font-weight: 500;
  color: rgba(255, 255, 255, 0.85);
  cursor: pointer;
  text-align: left;
  transition: background-color 0.15s ease, color 0.15s ease;
  letter-spacing: -0.0094em;
}
.styles-module__dropdownItem___ylsLj:hover {
  background: rgba(255, 255, 255, 0.08);
}
.styles-module__dropdownItem___ylsLj.styles-module__selected___OwRqP {
  background: rgba(255, 255, 255, 0.12);
  color: #fff;
  font-weight: 600;
}

.styles-module__settingsLabel___8UjfX {
  font-size: 0.8125rem;
  font-weight: 400;
  letter-spacing: -0.0094em;
  color: rgba(255, 255, 255, 0.5);
  display: flex;
  align-items: center;
  gap: 0.125rem;
}
[data-agentation-theme=light] .styles-module__settingsLabel___8UjfX {
  color: rgba(0, 0, 0, 0.5);
}

.styles-module__settingsLabelMarker___ewdtV {
  padding-top: 3px;
  margin-bottom: 10px;
}

.styles-module__settingsOptions___LyrBA {
  display: flex;
  gap: 0.25rem;
}

.styles-module__settingsOption___UNa12 {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.25rem;
  padding: 0.375rem 0.5rem;
  border: none;
  border-radius: 0.375rem;
  background: transparent;
  font-size: 0.6875rem;
  font-weight: 500;
  color: rgba(0, 0, 0, 0.7);
  cursor: pointer;
  transition: background-color 0.15s ease, color 0.15s ease;
}
.styles-module__settingsOption___UNa12:hover {
  background: rgba(0, 0, 0, 0.05);
}
.styles-module__settingsOption___UNa12.styles-module__selected___OwRqP {
  background: color-mix(in srgb, var(--agentation-color-blue) 15%, transparent);
  color: var(--agentation-color-blue);
}

.styles-module__sliderContainer___ducXj {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.styles-module__slider___GLdxp {
  -webkit-appearance: none;
  appearance: none;
  width: 100%;
  height: 4px;
  background: rgba(255, 255, 255, 0.15);
  border-radius: 2px;
  outline: none;
  cursor: pointer;
}
.styles-module__slider___GLdxp::-webkit-slider-thumb {
  -webkit-appearance: none;
  appearance: none;
  width: 14px;
  height: 14px;
  background: white;
  border-radius: 50%;
  cursor: pointer;
  transition: transform 0.15s ease, box-shadow 0.15s ease;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.3);
}
.styles-module__slider___GLdxp::-moz-range-thumb {
  width: 14px;
  height: 14px;
  background: white;
  border: none;
  border-radius: 50%;
  cursor: pointer;
  transition: transform 0.15s ease, box-shadow 0.15s ease;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.3);
}
.styles-module__slider___GLdxp:hover::-webkit-slider-thumb {
  transform: scale(1.15);
  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.4);
}
.styles-module__slider___GLdxp:hover::-moz-range-thumb {
  transform: scale(1.15);
  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.4);
}

.styles-module__sliderLabels___FhLDB {
  display: flex;
  justify-content: space-between;
}

.styles-module__sliderLabel___U8sPr {
  font-size: 0.625rem;
  font-weight: 500;
  color: rgba(255, 255, 255, 0.4);
  cursor: pointer;
  transition: color 0.15s ease;
}
.styles-module__sliderLabel___U8sPr:hover {
  color: rgba(255, 255, 255, 0.7);
}
.styles-module__sliderLabel___U8sPr.styles-module__active___-zoN6 {
  color: rgba(255, 255, 255, 0.9);
}

.styles-module__colorOptions___iHCNX {
  display: flex;
  gap: 0.5rem;
  margin-top: 0.375rem;
  margin-bottom: 1px;
}

.styles-module__colorOption___IodiY {
  display: block;
  width: 20px;
  height: 20px;
  border-radius: 50%;
  border: 2px solid transparent;
  background-color: var(--swatch);
  cursor: pointer;
  transition: transform 0.2s cubic-bezier(0.25, 1, 0.5, 1);
}
@supports (color: color(display-p3 0 0 0)) {
  .styles-module__colorOption___IodiY {
    background-color: var(--swatch-p3);
  }
}
.styles-module__colorOption___IodiY:hover {
  transform: scale(1.15);
}
.styles-module__colorOption___IodiY.styles-module__selected___OwRqP {
  transform: scale(0.83);
}

.styles-module__colorOptionRing___U2xpo {
  display: flex;
  width: 24px;
  height: 24px;
  border: 2px solid transparent;
  border-radius: 50%;
  transition: border-color 0.3s ease;
}
.styles-module__colorOptionRing___U2xpo.styles-module__selected___OwRqP {
  border-color: var(--swatch);
}
@supports (color: color(display-p3 0 0 0)) {
  .styles-module__colorOptionRing___U2xpo.styles-module__selected___OwRqP {
    border-color: var(--swatch-p3);
  }
}

.styles-module__settingsToggle___fBrFn {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  cursor: pointer;
}
.styles-module__settingsToggle___fBrFn + .styles-module__settingsToggle___fBrFn {
  margin-top: calc(0.5rem + 6px);
}
.styles-module__settingsToggle___fBrFn input[type=checkbox] {
  position: absolute;
  opacity: 0;
  width: 0;
  height: 0;
}
.styles-module__settingsToggle___fBrFn.styles-module__settingsToggleMarginBottom___MZUyF {
  margin-bottom: calc(0.5rem + 6px);
}

@keyframes styles-module__mcpPulse___uNggr {
  0% {
    box-shadow: 0 0 0 0 color-mix(in srgb, var(--agentation-color-green) 50%, transparent);
  }
  70% {
    box-shadow: 0 0 0 6px color-mix(in srgb, var(--agentation-color-green) 0%, transparent);
  }
  100% {
    box-shadow: 0 0 0 0 color-mix(in srgb, var(--agentation-color-green) 0%, transparent);
  }
}
@keyframes styles-module__mcpPulseError___fov9B {
  0% {
    box-shadow: 0 0 0 0 color-mix(in srgb, var(--agentation-color-red) 50%, transparent);
  }
  70% {
    box-shadow: 0 0 0 6px color-mix(in srgb, var(--agentation-color-red) 0%, transparent);
  }
  100% {
    box-shadow: 0 0 0 0 color-mix(in srgb, var(--agentation-color-red) 0%, transparent);
  }
}
.styles-module__mcpStatusDot___ibgkc {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}
.styles-module__mcpStatusDot___ibgkc.styles-module__connecting___uo-CW {
  background-color: var(--agentation-color-yellow);
  animation: styles-module__mcpPulse___uNggr 1.5s infinite;
}
.styles-module__mcpStatusDot___ibgkc.styles-module__connected___7c28g {
  background-color: var(--agentation-color-green);
  animation: styles-module__mcpPulse___uNggr 2.5s ease-in-out infinite;
}
.styles-module__mcpStatusDot___ibgkc.styles-module__disconnected___cHPxR {
  background-color: var(--agentation-color-red);
  animation: styles-module__mcpPulseError___fov9B 2s infinite;
}

.styles-module__drawCanvas___7cG9U {
  position: fixed;
  inset: 0;
  z-index: 99996;
  pointer-events: none !important;
}
.styles-module__drawCanvas___7cG9U.styles-module__active___-zoN6 {
  pointer-events: auto !important;
  cursor: crosshair !important;
}
.styles-module__drawCanvas___7cG9U.styles-module__active___-zoN6[data-stroke-hover] {
  cursor: pointer !important;
}

.styles-module__dragSelection___kZLq2 {
  position: fixed;
  top: 0;
  left: 0;
  border: 2px solid color-mix(in srgb, var(--agentation-color-green) 60%, transparent);
  border-radius: 4px;
  background-color: color-mix(in srgb, var(--agentation-color-green) 8%, transparent);
  pointer-events: none;
  z-index: 99997;
  will-change: transform, width, height;
  contain: layout style;
}

.styles-module__dragCount___KM90j {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  background-color: var(--agentation-color-green);
  color: white;
  font-size: 0.875rem;
  font-weight: 600;
  padding: 0.25rem 0.5rem;
  border-radius: 1rem;
  min-width: 1.5rem;
  text-align: center;
}

.styles-module__highlightsContainer___-0xzG {
  position: fixed;
  top: 0;
  left: 0;
  pointer-events: none;
  z-index: 99996;
}

.styles-module__selectedElementHighlight___fyVlI {
  position: fixed;
  top: 0;
  left: 0;
  border: 2px solid color-mix(in srgb, var(--agentation-color-green) 50%, transparent);
  border-radius: 4px;
  background: color-mix(in srgb, var(--agentation-color-green) 6%, transparent);
  pointer-events: none;
  will-change: transform, width, height;
  contain: layout style;
}

[data-agentation-theme=light] .styles-module__toolbarContainer___dIhma {
  background: #fff;
  color: rgba(0, 0, 0, 0.85);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08), 0 4px 16px rgba(0, 0, 0, 0.06), 0 0 0 1px rgba(0, 0, 0, 0.04);
}
[data-agentation-theme=light] .styles-module__toolbarContainer___dIhma.styles-module__collapsed___Rydsn:hover {
  background: #f5f5f5;
}
[data-agentation-theme=light] .styles-module__toggleContent___0yfyP {
  color: rgba(0, 0, 0, 0.85);
}
[data-agentation-theme=light] .styles-module__toggleContent___0yfyP.styles-module__expandedToggle___F7SRN {
  color: rgba(0, 0, 0, 0.5);
}
[data-agentation-theme=light] .styles-module__toggleContent___0yfyP.styles-module__expandedToggle___F7SRN:hover {
  color: rgba(0, 0, 0, 0.85);
}
[data-agentation-theme=light] .styles-module__toggleContent___0yfyP.styles-module__expandedToggle___F7SRN:hover::before {
  background: rgba(0, 0, 0, 0.06);
}
[data-agentation-theme=light] .styles-module__controlButton___8Q0jc {
  color: rgba(0, 0, 0, 0.5);
}
[data-agentation-theme=light] .styles-module__controlButton___8Q0jc:hover:not(:disabled):not([data-active=true]):not([data-failed=true]):not([data-auto-sync=true]):not([data-error=true]):not([data-no-hover=true]) {
  background: rgba(0, 0, 0, 0.06);
  color: rgba(0, 0, 0, 0.85);
}
[data-agentation-theme=light] .styles-module__controlButton___8Q0jc[data-active=true] {
  color: var(--agentation-color-blue);
  background: color-mix(in srgb, var(--agentation-color-blue) 15%, transparent);
}
[data-agentation-theme=light] .styles-module__controlButton___8Q0jc[data-error=true] {
  color: var(--agentation-color-red);
  background: color-mix(in srgb, var(--agentation-color-red) 15%, transparent);
}
[data-agentation-theme=light] .styles-module__controlButton___8Q0jc[data-danger]:hover:not(:disabled):not([data-active=true]):not([data-failed=true]) {
  color: var(--agentation-color-red);
  background: color-mix(in srgb, var(--agentation-color-red) 15%, transparent);
}
[data-agentation-theme=light] .styles-module__controlButton___8Q0jc[data-auto-sync=true] {
  color: var(--agentation-color-green);
  background: transparent;
}
[data-agentation-theme=light] .styles-module__controlButton___8Q0jc[data-failed=true] {
  color: var(--agentation-color-red);
  background: color-mix(in srgb, var(--agentation-color-red) 15%, transparent);
}
[data-agentation-theme=light] .styles-module__buttonTooltip___Burd9 {
  background: #fff;
  color: rgba(0, 0, 0, 0.85);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08), 0 4px 16px rgba(0, 0, 0, 0.06), 0 0 0 1px rgba(0, 0, 0, 0.04);
}
[data-agentation-theme=light] .styles-module__buttonTooltip___Burd9::after {
  background: #fff;
}
[data-agentation-theme=light] .styles-module__divider___c--s1 {
  background: rgba(0, 0, 0, 0.1);
}`,O={toolbar:"styles-module__toolbar___wNsdK",markersLayer:"styles-module__markersLayer___-25j1",fixedMarkersLayer:"styles-module__fixedMarkersLayer___ffyX6",controlsContent:"styles-module__controlsContent___9GJWU",disableTransitions:"styles-module__disableTransitions___EopxO",positionContext:"styles-module__positionContext___AZFHE",toolbarContainer:"styles-module__toolbarContainer___dIhma",entrance:"styles-module__entrance___sgHd8",toolbarEnter:"styles-module__toolbarEnter___u8RRu",hiding:"styles-module__hiding___1td44",toolbarHide:"styles-module__toolbarHide___y8kaT",collapsed:"styles-module__collapsed___Rydsn",expanded:"styles-module__expanded___ofKPx",serverConnected:"styles-module__serverConnected___Gfbou",buttonWrapper:"styles-module__buttonWrapper___rBcdv",toggleWrapper:"styles-module__toggleWrapper___7N0-q",togglePlaceholder:"styles-module__togglePlaceholder___wnqrL",toggleContent:"styles-module__toggleContent___0yfyP",expandedToggle:"styles-module__expandedToggle___F7SRN",toggleGlyph:"styles-module__toggleGlyph___R7Oom",toggleIcon:"styles-module__toggleIcon___Jbtus",toggleTopLine:"styles-module__toggleTopLine___hQaCm",toggleMiddleLine:"styles-module__toggleMiddleLine___sFFVe",toggleBottomLine:"styles-module__toggleBottomLine___V-jX3",toggleSparkle:"styles-module__toggleSparkle___eeF99",controlButton:"styles-module__controlButton___8Q0jc",visible:"styles-module__visible___KHwEW",hidden:"styles-module__hidden___Ae8H4",badge:"styles-module__badge___2XsgF",fadeOut:"styles-module__fadeOut___6Ut6-",badgeEnter:"styles-module__badgeEnter___mVQLj",statusShowing:"styles-module__statusShowing___te6iu",buttonBadge:"styles-module__buttonBadge___NeFWb",mcpIndicator:"styles-module__mcpIndicator___zGJeL",connected:"styles-module__connected___7c28g",mcpIndicatorPulseConnected:"styles-module__mcpIndicatorPulseConnected___EDodZ",connecting:"styles-module__connecting___uo-CW",mcpIndicatorPulseConnecting:"styles-module__mcpIndicatorPulseConnecting___cCYte",connectionIndicatorWrapper:"styles-module__connectionIndicatorWrapper___L-e-3",connectionIndicator:"styles-module__connectionIndicator___afk9p",connectionIndicatorVisible:"styles-module__connectionIndicatorVisible___C-i5B",connectionIndicatorConnected:"styles-module__connectionIndicatorConnected___IY8pR",connectionPulse:"styles-module__connectionPulse___-Zycw",connectionIndicatorDisconnected:"styles-module__connectionIndicatorDisconnected___kmpaZ",connectionIndicatorConnecting:"styles-module__connectionIndicatorConnecting___QmSLH",buttonTooltip:"styles-module__buttonTooltip___Burd9",tooltipsInSession:"styles-module__tooltipsInSession___-0lHH",sendButtonWrapper:"styles-module__sendButtonWrapper___UUxG6",sendButtonVisible:"styles-module__sendButtonVisible___WPSQU",shortcut:"styles-module__shortcut___lEAQk",tooltipBelow:"styles-module__tooltipBelow___m6ats",tooltipsHidden:"styles-module__tooltipsHidden___VtLJG",tooltipVisible:"styles-module__tooltipVisible___0jcCv",buttonWrapperAlignLeft:"styles-module__buttonWrapperAlignLeft___myzIp",buttonWrapperAlignRight:"styles-module__buttonWrapperAlignRight___HCQFR",divider:"styles-module__divider___c--s1",overlay:"styles-module__overlay___Q1O9y",hoverHighlight:"styles-module__hoverHighlight___ogakW",enter:"styles-module__enter___WFIki",hoverHighlightIn:"styles-module__hoverHighlightIn___6WYHY",multiSelectOutline:"styles-module__multiSelectOutline___cSJ-m",fadeIn:"styles-module__fadeIn___b9qmf",exit:"styles-module__exit___fyOJ0",singleSelectOutline:"styles-module__singleSelectOutline___QhX-O",hoverTooltip:"styles-module__hoverTooltip___bvLk7",hoverTooltipIn:"styles-module__hoverTooltipIn___FYGQx",hoverReactPath:"styles-module__hoverReactPath___gx1IJ",hoverElementName:"styles-module__hoverElementName___QMLMl",marker:"styles-module__marker___6sQrs",clearing:"styles-module__clearing___FQ--7",markerIn:"styles-module__markerIn___5FaAP",markerOut:"styles-module__markerOut___GU5jX",pending:"styles-module__pending___2IHLC",fixed:"styles-module__fixed___dBMHC",multiSelect:"styles-module__multiSelect___YWiuz",hovered:"styles-module__hovered___ZgXIy",renumber:"styles-module__renumber___nCTxD",renumberRoll:"styles-module__renumberRoll___Wgbq3",markerTooltip:"styles-module__markerTooltip___aLJID",tooltipIn:"styles-module__tooltipIn___0N31w",markerQuote:"styles-module__markerQuote___FHmrz",markerNote:"styles-module__markerNote___QkrrS",markerHint:"styles-module__markerHint___2iF-6",settingsPanel:"styles-module__settingsPanel___OxX3Y",settingsHeader:"styles-module__settingsHeader___pwDY9",settingsBrand:"styles-module__settingsBrand___0gJeM",settingsBrandSlash:"styles-module__settingsBrandSlash___uTG18",settingsVersion:"styles-module__settingsVersion___TUcFq",settingsSection:"styles-module__settingsSection___m-YM2",settingsLabel:"styles-module__settingsLabel___8UjfX",cycleButton:"styles-module__cycleButton___FMKfw",cycleDot:"styles-module__cycleDot___nPgLY",dropdownButton:"styles-module__dropdownButton___16NPz",toggleLabel:"styles-module__toggleLabel___Xm8Aa",customCheckbox:"styles-module__customCheckbox___U39ax",sliderLabel:"styles-module__sliderLabel___U8sPr",slider:"styles-module__slider___GLdxp",themeToggle:"styles-module__themeToggle___2rUjA",settingsOption:"styles-module__settingsOption___UNa12",selected:"styles-module__selected___OwRqP",settingsPanelContainer:"styles-module__settingsPanelContainer___Xksv8",settingsPage:"styles-module__settingsPage___6YfHH",slideLeft:"styles-module__slideLeft___Ps01J",automationsPage:"styles-module__automationsPage___uvCq6",slideIn:"styles-module__slideIn___4-qXe",settingsNavLink:"styles-module__settingsNavLink___wCzJt",settingsNavLinkRight:"styles-module__settingsNavLinkRight___ZWwhj",mcpNavIndicator:"styles-module__mcpNavIndicator___cl9pO",mcpPulse:"styles-module__mcpPulse___uNggr",settingsBackButton:"styles-module__settingsBackButton___bIe2j",automationHeader:"styles-module__automationHeader___InP0r",automationDescription:"styles-module__automationDescription___NKlmo",learnMoreLink:"styles-module__learnMoreLink___8xv-x",autoSendRow:"styles-module__autoSendRow___UblX5",autoSendLabel:"styles-module__autoSendLabel___icDc2",active:"styles-module__active___-zoN6",webhookUrlInput:"styles-module__webhookUrlInput___2375C",settingsSectionExtraPadding:"styles-module__settingsSectionExtraPadding___jdhFV",settingsSectionGrow:"styles-module__settingsSectionGrow___h-5HZ",settingsRow:"styles-module__settingsRow___3sdhc",settingsRowMarginTop:"styles-module__settingsRowMarginTop___zA0Sp",dropdownContainer:"styles-module__dropdownContainer___BVnxe",settingsRowDisabled:"styles-module__settingsRowDisabled___EgS0V",toggleSwitch:"styles-module__toggleSwitch___l4Ygm",cycleButtonText:"styles-module__cycleButtonText___fD1LR",cycleTextIn:"styles-module__cycleTextIn___Q6zJf",cycleDots:"styles-module__cycleDots___LWuoQ",dropdownMenu:"styles-module__dropdownMenu___k73ER",scaleIn:"styles-module__scaleIn___c-r1K",dropdownItem:"styles-module__dropdownItem___ylsLj",settingsLabelMarker:"styles-module__settingsLabelMarker___ewdtV",settingsOptions:"styles-module__settingsOptions___LyrBA",sliderContainer:"styles-module__sliderContainer___ducXj",sliderLabels:"styles-module__sliderLabels___FhLDB",colorOptions:"styles-module__colorOptions___iHCNX",colorOption:"styles-module__colorOption___IodiY",colorOptionRing:"styles-module__colorOptionRing___U2xpo",settingsToggle:"styles-module__settingsToggle___fBrFn",settingsToggleMarginBottom:"styles-module__settingsToggleMarginBottom___MZUyF",mcpStatusDot:"styles-module__mcpStatusDot___ibgkc",disconnected:"styles-module__disconnected___cHPxR",mcpPulseError:"styles-module__mcpPulseError___fov9B",drawCanvas:"styles-module__drawCanvas___7cG9U",dragSelection:"styles-module__dragSelection___kZLq2",dragCount:"styles-module__dragCount___KM90j",highlightsContainer:"styles-module__highlightsContainer___-0xzG",selectedElementHighlight:"styles-module__selectedElementHighlight___fyVlI",scaleOut:"styles-module__scaleOut___Wctwz",slideUp:"styles-module__slideUp___kgD36",slideDown:"styles-module__slideDown___zcdje"};function P2({active:e}){return T2("svg",{className:O.toggleGlyph,"data-active":e,width:"24",height:"24",viewBox:"0 0 24 24",fill:"none",stroke:"currentColor",strokeWidth:"1.5",strokeLinecap:"round",strokeLinejoin:"round","aria-hidden":"true",children:[hc("path",{className:O.toggleTopLine,d:"M5.5 6.75H18.5"}),hc("path",{className:O.toggleMiddleLine,d:"M5.5 12H11.5"}),hc("path",{className:O.toggleBottomLine,d:"M5.5 17.25H9.25"}),hc("path",{className:O.toggleSparkle,d:"M16 12.75L16.5179 13.9677C16.8078 14.6494 17.3506 15.1922 18.0323 15.4821L19.25 16L18.0323 16.5179C17.3506 16.8078 16.8078 17.3506 16.5179 18.0323L16 19.25L15.4821 18.0323C15.1922 17.3506 14.6494 16.8078 13.9677 16.5179L12.75 16L13.9677 15.4821C14.6494 15.1922 15.1922 14.6494 15.4821 13.9677L16 12.75Z"})]})}var ne={navigation:{width:800,height:56},hero:{width:800,height:320},header:{width:800,height:80},section:{width:800,height:400},sidebar:{width:240,height:400},footer:{width:800,height:160},modal:{width:480,height:300},card:{width:280,height:240},text:{width:400,height:120},image:{width:320,height:200},video:{width:480,height:270},table:{width:560,height:220},grid:{width:600,height:300},list:{width:300,height:180},chart:{width:400,height:240},button:{width:140,height:40},input:{width:280,height:56},form:{width:360,height:320},tabs:{width:480,height:240},dropdown:{width:200,height:200},toggle:{width:44,height:24},search:{width:320,height:44},avatar:{width:48,height:48},badge:{width:80,height:28},breadcrumb:{width:300,height:24},pagination:{width:300,height:36},progress:{width:240,height:8},divider:{width:600,height:1},accordion:{width:400,height:200},carousel:{width:600,height:300},toast:{width:320,height:64},tooltip:{width:180,height:40},pricing:{width:300,height:360},testimonial:{width:360,height:200},cta:{width:600,height:160},alert:{width:400,height:56},banner:{width:800,height:48},stat:{width:200,height:120},stepper:{width:480,height:48},tag:{width:72,height:28},rating:{width:160,height:28},map:{width:480,height:300},timeline:{width:360,height:320},fileUpload:{width:360,height:180},codeBlock:{width:480,height:200},calendar:{width:300,height:300},notification:{width:360,height:72},productCard:{width:280,height:360},profile:{width:280,height:200},drawer:{width:320,height:400},popover:{width:240,height:160},logo:{width:120,height:40},faq:{width:560,height:320},gallery:{width:560,height:360},checkbox:{width:20,height:20},radio:{width:20,height:20},slider:{width:240,height:32},datePicker:{width:300,height:320},skeleton:{width:320,height:120},chip:{width:96,height:32},icon:{width:24,height:24},spinner:{width:32,height:32},feature:{width:360,height:200},team:{width:560,height:280},login:{width:360,height:360},contact:{width:400,height:320}},Ig=[{section:"Layout",items:[{type:"navigation",label:"Navigation",...ne.navigation},{type:"header",label:"Header",...ne.header},{type:"hero",label:"Hero",...ne.hero},{type:"section",label:"Section",...ne.section},{type:"sidebar",label:"Sidebar",...ne.sidebar},{type:"footer",label:"Footer",...ne.footer},{type:"modal",label:"Modal",...ne.modal},{type:"banner",label:"Banner",...ne.banner},{type:"drawer",label:"Drawer",...ne.drawer},{type:"popover",label:"Popover",...ne.popover},{type:"divider",label:"Divider",...ne.divider}]},{section:"Content",items:[{type:"card",label:"Card",...ne.card},{type:"text",label:"Text",...ne.text},{type:"image",label:"Image",...ne.image},{type:"video",label:"Video",...ne.video},{type:"table",label:"Table",...ne.table},{type:"grid",label:"Grid",...ne.grid},{type:"list",label:"List",...ne.list},{type:"chart",label:"Chart",...ne.chart},{type:"codeBlock",label:"Code Block",...ne.codeBlock},{type:"map",label:"Map",...ne.map},{type:"timeline",label:"Timeline",...ne.timeline},{type:"calendar",label:"Calendar",...ne.calendar},{type:"accordion",label:"Accordion",...ne.accordion},{type:"carousel",label:"Carousel",...ne.carousel},{type:"logo",label:"Logo",...ne.logo},{type:"faq",label:"FAQ",...ne.faq},{type:"gallery",label:"Gallery",...ne.gallery}]},{section:"Controls",items:[{type:"button",label:"Button",...ne.button},{type:"input",label:"Input",...ne.input},{type:"search",label:"Search",...ne.search},{type:"form",label:"Form",...ne.form},{type:"tabs",label:"Tabs",...ne.tabs},{type:"dropdown",label:"Dropdown",...ne.dropdown},{type:"toggle",label:"Toggle",...ne.toggle},{type:"stepper",label:"Stepper",...ne.stepper},{type:"rating",label:"Rating",...ne.rating},{type:"fileUpload",label:"File Upload",...ne.fileUpload},{type:"checkbox",label:"Checkbox",...ne.checkbox},{type:"radio",label:"Radio",...ne.radio},{type:"slider",label:"Slider",...ne.slider},{type:"datePicker",label:"Date Picker",...ne.datePicker}]},{section:"Elements",items:[{type:"avatar",label:"Avatar",...ne.avatar},{type:"badge",label:"Badge",...ne.badge},{type:"tag",label:"Tag",...ne.tag},{type:"breadcrumb",label:"Breadcrumb",...ne.breadcrumb},{type:"pagination",label:"Pagination",...ne.pagination},{type:"progress",label:"Progress",...ne.progress},{type:"alert",label:"Alert",...ne.alert},{type:"toast",label:"Toast",...ne.toast},{type:"notification",label:"Notification",...ne.notification},{type:"tooltip",label:"Tooltip",...ne.tooltip},{type:"stat",label:"Stat",...ne.stat},{type:"skeleton",label:"Skeleton",...ne.skeleton},{type:"chip",label:"Chip",...ne.chip},{type:"icon",label:"Icon",...ne.icon},{type:"spinner",label:"Spinner",...ne.spinner}]},{section:"Blocks",items:[{type:"pricing",label:"Pricing",...ne.pricing},{type:"testimonial",label:"Testimonial",...ne.testimonial},{type:"cta",label:"CTA",...ne.cta},{type:"productCard",label:"Product Card",...ne.productCard},{type:"profile",label:"Profile",...ne.profile},{type:"feature",label:"Feature",...ne.feature},{type:"team",label:"Team",...ne.team},{type:"login",label:"Login",...ne.login},{type:"contact",label:"Contact",...ne.contact}]}],po={};for(let e of Ig)for(let t of e.items)po[t.type]=t;function A({w:e,h:t=3,strong:n}){return y("div",{style:{width:typeof e=="number"?`${e}px`:e,height:t,borderRadius:2,background:n?"var(--agd-bar-strong)":"var(--agd-bar)",flexShrink:0}})}function ut({w:e,h:t,radius:n=3,style:o}){return y("div",{style:{width:typeof e=="number"?`${e}px`:e,height:typeof t=="number"?`${t}px`:t,borderRadius:n,border:"1px dashed var(--agd-stroke)",background:"var(--agd-fill)",flexShrink:0,...o}})}function vn({size:e}){return y("div",{style:{width:e,height:e,borderRadius:"50%",border:"1px dashed var(--agd-stroke)",background:"var(--agd-fill)",flexShrink:0}})}function D2({width:e,height:t}){let n=Math.max(8,t*.2);return W("div",{style:{display:"flex",alignItems:"center",height:"100%",padding:`0 ${n}px`,gap:e*.02},children:[y(ut,{w:Math.max(20,t*.5),h:Math.max(12,t*.4),radius:2}),W("div",{style:{flex:1,display:"flex",gap:e*.03,marginLeft:e*.04},children:[y(A,{w:e*.06}),y(A,{w:e*.07}),y(A,{w:e*.05}),y(A,{w:e*.06})]}),y(ut,{w:e*.1,h:Math.min(28,t*.5),radius:4})]})}function B2({width:e,height:t,text:n}){return W("div",{style:{display:"flex",flexDirection:"column",alignItems:"center",justifyContent:"center",height:"100%",gap:t*.05},children:[n?y("span",{style:{fontSize:Math.min(20,t*.08),fontWeight:600,color:"var(--agd-text-3)",textAlign:"center",maxWidth:"80%"},children:n}):y(A,{w:e*.5,h:Math.max(6,t*.04),strong:!0}),y(A,{w:e*.6}),y(A,{w:e*.4}),y(ut,{w:Math.min(140,e*.2),h:Math.min(36,t*.12),radius:6,style:{marginTop:t*.06}})]})}function z2({width:e,height:t}){let n=Math.max(3,Math.floor(t/36));return W("div",{style:{padding:e*.08,display:"flex",flexDirection:"column",gap:t*.03},children:[y(A,{w:e*.6,h:4,strong:!0}),Array.from({length:n},(o,r)=>W("div",{style:{display:"flex",alignItems:"center",gap:6},children:[y(ut,{w:10,h:10,radius:2}),y(A,{w:e*(.4+r*17%30/100)})]},r))]})}function O2({width:e,height:t}){let n=Math.max(2,Math.min(4,Math.floor(e/160)));return y("div",{style:{display:"flex",padding:`${t*.12}px ${e*.03}px`,gap:e*.05},children:Array.from({length:n},(o,r)=>W("div",{style:{flex:1,display:"flex",flexDirection:"column",gap:4},children:[y(A,{w:"60%",h:3,strong:!0}),y(A,{w:"80%",h:2}),y(A,{w:"70%",h:2}),y(A,{w:"60%",h:2})]},r))})}function A2({width:e,height:t}){return W("div",{style:{height:"100%",display:"flex",flexDirection:"column"},children:[W("div",{style:{padding:"10px 12px",borderBottom:"1px solid var(--agd-stroke)",display:"flex",alignItems:"center",justifyContent:"space-between"},children:[y(A,{w:e*.3,h:4,strong:!0}),y("div",{style:{width:14,height:14,border:"1px solid var(--agd-stroke)",borderRadius:3}})]}),W("div",{style:{flex:1,padding:12,display:"flex",flexDirection:"column",gap:6},children:[y(A,{w:"90%"}),y(A,{w:"70%"}),y(A,{w:"80%"})]}),W("div",{style:{padding:"10px 12px",borderTop:"1px solid var(--agd-stroke)",display:"flex",justifyContent:"flex-end",gap:8},children:[y(ut,{w:70,h:26,radius:4}),y(ut,{w:70,h:26,radius:4,style:{background:"var(--agd-bar)"}})]})]})}function F2({width:e,height:t}){return W("div",{style:{height:"100%",display:"flex",flexDirection:"column"},children:[y("div",{style:{height:"40%",background:"var(--agd-fill)",borderBottom:"1px dashed var(--agd-stroke)"}}),W("div",{style:{flex:1,padding:10,display:"flex",flexDirection:"column",gap:5},children:[y(A,{w:"70%",h:4,strong:!0}),y(A,{w:"95%",h:2}),y(A,{w:"85%",h:2}),y(A,{w:"50%",h:2})]})]})}function W2({width:e,height:t,text:n}){if(n)return y("div",{style:{padding:4,fontSize:Math.min(14,t*.3),lineHeight:1.5,color:"var(--agd-text-3)",wordBreak:"break-word",overflow:"hidden"},children:n});let o=Math.max(2,Math.floor(t/18));return W("div",{style:{display:"flex",flexDirection:"column",gap:6,padding:4},children:[y(A,{w:e*.6,h:5,strong:!0}),Array.from({length:o},(r,i)=>y(A,{w:`${70+i*13%25}%`,h:2},i))]})}function j2({width:e,height:t}){return y("div",{style:{height:"100%",position:"relative"},children:W("svg",{width:"100%",height:"100%",viewBox:`0 0 ${e} ${t}`,preserveAspectRatio:"none",fill:"none",children:[y("line",{x1:"0",y1:"0",x2:e,y2:t,stroke:"var(--agd-stroke)",strokeWidth:"1"}),y("line",{x1:e,y1:"0",x2:"0",y2:t,stroke:"var(--agd-stroke)",strokeWidth:"1"}),y("circle",{cx:e*.3,cy:t*.3,r:Math.min(e,t)*.08,fill:"var(--agd-fill)",stroke:"var(--agd-stroke)",strokeWidth:"0.8"})]})})}function H2({width:e,height:t}){let n=Math.max(2,Math.min(5,Math.floor(e/100))),o=Math.max(2,Math.min(6,Math.floor(t/32)));return W("div",{style:{height:"100%",display:"flex",flexDirection:"column"},children:[y("div",{style:{display:"flex",borderBottom:"1px solid var(--agd-stroke)",padding:"6px 0"},children:Array.from({length:n},(r,i)=>y("div",{style:{flex:1,padding:"0 8px"},children:y(A,{w:"70%",h:3,strong:!0})},i))}),Array.from({length:o},(r,i)=>y("div",{style:{display:"flex",borderBottom:"1px solid rgba(255,255,255,0.03)",padding:"6px 0"},children:Array.from({length:n},(l,s)=>y("div",{style:{flex:1,padding:"0 8px"},children:y(A,{w:`${50+(i*7+s*13)%40}%`,h:2})},s))},i))]})}function U2({width:e,height:t}){let n=Math.max(2,Math.floor(t/28));return y("div",{style:{display:"flex",flexDirection:"column",gap:4,padding:4},children:Array.from({length:n},(o,r)=>W("div",{style:{display:"flex",alignItems:"center",gap:8,padding:"4px 0"},children:[y(vn,{size:8}),y(A,{w:`${55+r*17%35}%`,h:2})]},r))})}function Y2({width:e,height:t,text:n}){return y("div",{style:{height:"100%",borderRadius:Math.min(8,t/3),border:"1px solid var(--agd-stroke)",background:"var(--agd-fill)",display:"flex",alignItems:"center",justifyContent:"center"},children:n?y("span",{style:{fontSize:Math.min(13,t*.4),fontWeight:500,color:"var(--agd-text-3)",letterSpacing:"-0.01em"},children:n}):y(A,{w:Math.max(20,e*.5),h:3,strong:!0})})}function Q2({width:e,height:t}){return W("div",{style:{display:"flex",flexDirection:"column",gap:4,height:"100%",justifyContent:"center"},children:[y(A,{w:Math.min(80,e*.3),h:2}),y("div",{style:{height:Math.min(36,t*.6),borderRadius:4,border:"1px dashed var(--agd-stroke)",background:"var(--agd-fill)",display:"flex",alignItems:"center",paddingLeft:8},children:y(A,{w:"40%",h:2})})]})}function V2({width:e,height:t}){let n=Math.max(2,Math.min(5,Math.floor(t/56)));return W("div",{style:{display:"flex",flexDirection:"column",gap:t*.04,padding:8},children:[Array.from({length:n},(o,r)=>W("div",{style:{display:"flex",flexDirection:"column",gap:4},children:[y(A,{w:60+r*17%30,h:2}),y(ut,{w:"100%",h:28,radius:4})]},r)),y(ut,{w:Math.min(120,e*.35),h:30,radius:6,style:{marginTop:8,alignSelf:"flex-end",background:"var(--agd-bar)"}})]})}function X2({width:e,height:t}){let n=Math.max(2,Math.min(4,Math.floor(e/120)));return W("div",{style:{height:"100%",display:"flex",flexDirection:"column"},children:[y("div",{style:{display:"flex",gap:2,borderBottom:"1px solid var(--agd-stroke)"},children:Array.from({length:n},(o,r)=>y("div",{style:{padding:"8px 12px",borderBottom:r===0?"2px solid var(--agd-bar-strong)":"none"},children:y(A,{w:60,h:3,strong:r===0})},r))}),W("div",{style:{flex:1,padding:12,display:"flex",flexDirection:"column",gap:6},children:[y(A,{w:"80%",h:2}),y(A,{w:"65%",h:2}),y(A,{w:"75%",h:2})]})]})}function G2({width:e,height:t}){let n=Math.min(e,t)/2;return W("svg",{width:"100%",height:"100%",viewBox:`0 0 ${e} ${t}`,fill:"none",children:[y("circle",{cx:e/2,cy:t/2,r:n-1,stroke:"var(--agd-stroke)",fill:"var(--agd-fill)",strokeWidth:"1.5",strokeDasharray:"3 2"}),y("circle",{cx:e/2,cy:t*.38,r:n*.28,stroke:"var(--agd-stroke)",fill:"var(--agd-fill)",strokeWidth:"0.8"}),y("path",{d:`M${e/2-n*.55} ${t*.78} C${e/2-n*.55} ${t*.55} ${e/2+n*.55} ${t*.55} ${e/2+n*.55} ${t*.78}`,stroke:"var(--agd-stroke)",fill:"var(--agd-fill)",strokeWidth:"0.8"})]})}function q2({width:e,height:t}){return y("div",{style:{height:"100%",borderRadius:t/2,border:"1px solid var(--agd-stroke)",background:"var(--agd-fill)",display:"flex",alignItems:"center",justifyContent:"center"},children:y(A,{w:Math.max(16,e*.5),h:2,strong:!0})})}function K2({width:e,height:t}){return W("div",{style:{display:"flex",flexDirection:"column",alignItems:"center",justifyContent:"center",height:"100%",gap:t*.08},children:[y(A,{w:e*.5,h:Math.max(5,t*.06),strong:!0}),y(A,{w:e*.35})]})}function J2({width:e,height:t}){return W("div",{style:{display:"flex",flexDirection:"column",height:"100%",gap:t*.04,padding:e*.04},children:[y(A,{w:e*.3,h:4,strong:!0}),y(A,{w:e*.7}),y(A,{w:e*.5}),W("div",{style:{flex:1,display:"flex",gap:e*.03,marginTop:t*.06},children:[y(ut,{w:"33%",h:"100%",radius:4}),y(ut,{w:"33%",h:"100%",radius:4}),y(ut,{w:"33%",h:"100%",radius:4})]})]})}function Z2({width:e,height:t}){let n=Math.max(2,Math.min(4,Math.floor(e/140))),o=Math.max(1,Math.min(3,Math.floor(t/120)));return y("div",{style:{display:"grid",gridTemplateColumns:`repeat(${n}, 1fr)`,gridTemplateRows:`repeat(${o}, 1fr)`,gap:6,height:"100%"},children:Array.from({length:n*o},(r,i)=>y(ut,{w:"100%",h:"100%",radius:4},i))})}function ex({width:e,height:t}){let n=Math.max(2,Math.floor((t-32)/28));return W("div",{style:{height:"100%",display:"flex",flexDirection:"column"},children:[y("div",{style:{padding:"6px 8px",borderBottom:"1px solid var(--agd-stroke)"},children:y(A,{w:e*.5,h:3,strong:!0})}),y("div",{style:{flex:1,padding:4,display:"flex",flexDirection:"column",gap:2},children:Array.from({length:n},(o,r)=>y("div",{style:{padding:"4px 6px",borderRadius:3,background:r===0?"var(--agd-fill)":"transparent"},children:y(A,{w:`${50+r*17%35}%`,h:2,strong:r===0})},r))})]})}function tx({width:e,height:t}){let n=Math.min(e,t)/2;return W("svg",{width:"100%",height:"100%",viewBox:`0 0 ${e} ${t}`,fill:"none",children:[y("rect",{x:"1",y:"1",width:e-2,height:t-2,rx:n,stroke:"var(--agd-stroke)",strokeWidth:"1"}),y("circle",{cx:e-n,cy:t/2,r:n*.7,fill:"var(--agd-bar)"})]})}function nx({width:e,height:t}){let n=Math.min(t/2,20);return W("div",{style:{height:"100%",borderRadius:n,border:"1px dashed var(--agd-stroke)",background:"var(--agd-fill)",display:"flex",alignItems:"center",padding:`0 ${n*.6}px`,gap:6},children:[y(vn,{size:Math.min(14,t*.4)}),y(A,{w:"50%",h:2})]})}function ox({width:e,height:t}){return W("div",{style:{height:"100%",borderRadius:8,border:"1px dashed var(--agd-stroke)",background:"var(--agd-fill)",display:"flex",alignItems:"center",padding:"0 10px",gap:8},children:[y(vn,{size:Math.min(20,t*.5)}),W("div",{style:{flex:1,display:"flex",flexDirection:"column",gap:3},children:[y(A,{w:"60%",h:3,strong:!0}),y(A,{w:"80%",h:2})]}),y("div",{style:{width:14,height:14,border:"1px solid var(--agd-stroke)",borderRadius:3,flexShrink:0}})]})}function rx({width:e,height:t}){return W("svg",{width:"100%",height:"100%",viewBox:`0 0 ${e} ${t}`,fill:"none",children:[y("rect",{x:"0",y:"0",width:e,height:t,rx:t/2,stroke:"var(--agd-stroke)",strokeWidth:"0.8"}),y("rect",{x:"1",y:"1",width:e*.65,height:t-2,rx:(t-2)/2,fill:"var(--agd-bar)"})]})}function ix({width:e,height:t}){let n=Math.max(3,Math.min(7,Math.floor(e/50))),o=e/(n*2);return y("div",{style:{height:"100%",display:"flex",alignItems:"flex-end",justifyContent:"space-around",padding:"0 4px",borderBottom:"1px solid var(--agd-stroke)"},children:Array.from({length:n},(r,i)=>{let l=30+(i*37+17)%55;return y(ut,{w:o,h:`${l}%`,radius:2},i)})})}function lx({width:e,height:t}){let n=Math.min(e,t)*.12;return W("div",{style:{height:"100%",position:"relative",display:"flex",alignItems:"center",justifyContent:"center"},children:[y(ut,{w:"100%",h:"100%",radius:4}),y("div",{style:{position:"absolute",width:n*2,height:n*2,borderRadius:"50%",border:"1.5px solid var(--agd-stroke)",background:"var(--agd-fill)",display:"flex",alignItems:"center",justifyContent:"center"},children:y("div",{style:{width:0,height:0,borderLeft:`${n*.6}px solid var(--agd-bar-strong)`,borderTop:`${n*.4}px solid transparent`,borderBottom:`${n*.4}px solid transparent`,marginLeft:n*.15}})})]})}function sx({width:e,height:t}){return W("div",{style:{height:"100%",display:"flex",flexDirection:"column",alignItems:"center"},children:[y("div",{style:{flex:1,width:"100%",borderRadius:6,border:"1px dashed var(--agd-stroke)",background:"var(--agd-fill)",display:"flex",alignItems:"center",justifyContent:"center"},children:y(A,{w:"60%",h:2})}),y("div",{style:{width:8,height:8,background:"var(--agd-fill)",border:"1px dashed var(--agd-stroke)",borderTop:"none",borderLeft:"none",transform:"rotate(45deg)",marginTop:-5}})]})}function ax({width:e,height:t}){let n=Math.max(2,Math.min(4,Math.floor(e/80)));return y("div",{style:{display:"flex",alignItems:"center",height:"100%",gap:4},children:Array.from({length:n},(o,r)=>W("div",{style:{display:"flex",alignItems:"center",gap:4},children:[r>0&&y("span",{style:{color:"var(--agd-stroke)",fontSize:10},children:"/"}),y(A,{w:40+r*13%20,h:2,strong:r===n-1})]},r))})}function cx({width:e,height:t}){let n=Math.max(3,Math.min(5,Math.floor(e/40))),o=Math.min(28,t*.8);return y("div",{style:{display:"flex",alignItems:"center",justifyContent:"center",height:"100%",gap:4},children:Array.from({length:n},(r,i)=>y(ut,{w:o,h:o,radius:4,style:i===1?{background:"var(--agd-bar)"}:void 0},i))})}function dx({width:e}){return y("div",{style:{display:"flex",alignItems:"center",height:"100%"},children:y("div",{style:{width:"100%",height:1,background:"var(--agd-stroke)"}})})}function ux({width:e,height:t}){let n=Math.max(2,Math.min(4,Math.floor(t/40)));return y("div",{style:{display:"flex",flexDirection:"column",height:"100%"},children:Array.from({length:n},(o,r)=>W("div",{style:{borderBottom:"1px solid var(--agd-stroke)",padding:"8px 6px",display:"flex",alignItems:"center",justifyContent:"space-between",flex:r===0?2:1},children:[y(A,{w:`${40+r*17%25}%`,h:3,strong:!0}),y("span",{style:{fontSize:8,color:"var(--agd-stroke)"},children:r===0?"\u25BC":"\u25B6"})]},r))})}function _x({width:e,height:t}){return W("div",{style:{height:"100%",display:"flex",flexDirection:"column",gap:6},children:[W("div",{style:{flex:1,display:"flex",gap:6,alignItems:"center"},children:[y("span",{style:{fontSize:12,color:"var(--agd-stroke)"},children:"\u2039"}),y(ut,{w:"100%",h:"100%",radius:4}),y("span",{style:{fontSize:12,color:"var(--agd-stroke)"},children:"\u203A"})]}),W("div",{style:{display:"flex",justifyContent:"center",gap:4},children:[y(vn,{size:5}),y(vn,{size:5}),y(vn,{size:5})]})]})}function fx({width:e,height:t}){return W("div",{style:{height:"100%",display:"flex",flexDirection:"column",alignItems:"center",padding:10,gap:t*.04},children:[y(A,{w:e*.4,h:3,strong:!0}),y(A,{w:e*.3,h:6,strong:!0}),y("div",{style:{flex:1,display:"flex",flexDirection:"column",gap:4,width:"100%",padding:"8px 0"},children:Array.from({length:4},(n,o)=>W("div",{style:{display:"flex",alignItems:"center",gap:4},children:[y(vn,{size:5}),y(A,{w:`${50+o*17%35}%`,h:2})]},o))}),y(ut,{w:e*.7,h:Math.min(32,t*.1),radius:6,style:{background:"var(--agd-bar)"}})]})}function hx({width:e,height:t}){return W("div",{style:{height:"100%",display:"flex",flexDirection:"column",padding:10,gap:8},children:[y("span",{style:{fontSize:18,lineHeight:1,color:"var(--agd-stroke)",fontFamily:"serif"},children:"\u201C"}),W("div",{style:{flex:1,display:"flex",flexDirection:"column",gap:4},children:[y(A,{w:"90%",h:2}),y(A,{w:"75%",h:2}),y(A,{w:"60%",h:2})]}),W("div",{style:{display:"flex",alignItems:"center",gap:6},children:[y(vn,{size:20}),W("div",{style:{display:"flex",flexDirection:"column",gap:2},children:[y(A,{w:60,h:3,strong:!0}),y(A,{w:40,h:2})]})]})]})}function px({width:e,height:t}){return W("div",{style:{display:"flex",flexDirection:"column",alignItems:"center",justifyContent:"center",height:"100%",gap:t*.08},children:[y(A,{w:e*.5,h:Math.max(4,t*.05),strong:!0}),y(A,{w:e*.35}),y(ut,{w:Math.min(140,e*.25),h:Math.min(32,t*.15),radius:6,style:{marginTop:t*.04,background:"var(--agd-bar)"}})]})}function mx({width:e,height:t}){return W("div",{style:{height:"100%",borderRadius:6,border:"1px dashed var(--agd-stroke)",background:"var(--agd-fill)",display:"flex",alignItems:"center",padding:"0 10px",gap:8},children:[y("div",{style:{width:16,height:16,borderRadius:"50%",border:"1.5px solid var(--agd-bar-strong)",display:"flex",alignItems:"center",justifyContent:"center",flexShrink:0},children:y("div",{style:{width:2,height:6,background:"var(--agd-bar-strong)",borderRadius:1}})}),W("div",{style:{flex:1,display:"flex",flexDirection:"column",gap:3},children:[y(A,{w:"40%",h:3,strong:!0}),y(A,{w:"70%",h:2})]})]})}function gx({width:e,height:t}){return W("div",{style:{height:"100%",background:"var(--agd-fill)",display:"flex",alignItems:"center",justifyContent:"center",gap:8,padding:"0 12px"},children:[y(A,{w:e*.4,h:3,strong:!0}),y(ut,{w:60,h:Math.min(24,t*.6),radius:4})]})}function yx({width:e,height:t}){return W("div",{style:{height:"100%",display:"flex",flexDirection:"column",alignItems:"center",justifyContent:"center",gap:t*.06},children:[y(A,{w:e*.5,h:2}),y(A,{w:e*.4,h:Math.max(8,t*.18),strong:!0}),y(A,{w:e*.3,h:2})]})}function xx({width:e,height:t}){let n=Math.max(3,Math.min(5,Math.floor(e/100))),o=Math.min(12,t*.35);return y("div",{style:{display:"flex",alignItems:"center",justifyContent:"space-between",height:"100%",padding:"0 8px"},children:Array.from({length:n},(r,i)=>W("div",{style:{display:"flex",alignItems:"center",gap:0,flex:1},children:[y("div",{style:{width:o,height:o,borderRadius:"50%",border:"1.5px solid var(--agd-stroke)",background:i===0?"var(--agd-bar)":"transparent",flexShrink:0}}),i<n-1&&y("div",{style:{flex:1,height:1,background:"var(--agd-stroke)",margin:"0 4px"}})]},i))})}function vx({width:e,height:t}){return W("div",{style:{height:"100%",borderRadius:4,border:"1px solid var(--agd-stroke)",background:"var(--agd-fill)",display:"flex",alignItems:"center",justifyContent:"center",gap:4,padding:"0 6px"},children:[y(A,{w:Math.max(16,e*.5),h:2,strong:!0}),y("div",{style:{width:8,height:8,borderRadius:"50%",border:"1px solid var(--agd-stroke)",flexShrink:0}})]})}function wx({width:e,height:t}){let o=Math.min(t*.7,e/7.5);return y("div",{style:{display:"flex",alignItems:"center",justifyContent:"center",height:"100%",gap:o*.2},children:Array.from({length:5},(r,i)=>y("svg",{width:o,height:o,viewBox:"0 0 16 16",fill:"none",children:y("path",{d:"M8 1.5l2 4 4.5.7-3.25 3.1.75 4.5L8 11.4l-4 2.4.75-4.5L1.5 6.2 6 5.5z",stroke:"var(--agd-stroke)",strokeWidth:"0.8",fill:i<3?"var(--agd-bar)":"none"})},i))})}function bx({width:e,height:t}){return W("div",{style:{height:"100%",position:"relative",borderRadius:4,border:"1px dashed var(--agd-stroke)",background:"var(--agd-fill)",overflow:"hidden"},children:[W("svg",{width:"100%",height:"100%",viewBox:`0 0 ${e} ${t}`,fill:"none",style:{position:"absolute",inset:0},children:[y("line",{x1:0,y1:t*.3,x2:e,y2:t*.7,stroke:"var(--agd-stroke)",strokeWidth:"0.5",opacity:".2"}),y("line",{x1:0,y1:t*.6,x2:e,y2:t*.2,stroke:"var(--agd-stroke)",strokeWidth:"0.5",opacity:".15"}),y("line",{x1:e*.4,y1:0,x2:e*.6,y2:t,stroke:"var(--agd-stroke)",strokeWidth:"0.5",opacity:".15"})]}),y("div",{style:{position:"absolute",left:"50%",top:"40%",transform:"translate(-50%, -100%)"},children:W("svg",{width:"16",height:"22",viewBox:"0 0 16 22",fill:"none",children:[y("path",{d:"M8 0C3.6 0 0 3.6 0 8c0 6 8 14 8 14s8-8 8-14c0-4.4-3.6-8-8-8z",fill:"var(--agd-bar)",opacity:".4"}),y("circle",{cx:"8",cy:"8",r:"3",fill:"var(--agd-fill)"})]})})]})}function kx({width:e,height:t}){let n=Math.max(3,Math.min(5,Math.floor(t/60)));return W("div",{style:{display:"flex",height:"100%",padding:"8px 0"},children:[y("div",{style:{width:16,display:"flex",flexDirection:"column",alignItems:"center"},children:Array.from({length:n},(o,r)=>W("div",{style:{display:"flex",flexDirection:"column",alignItems:"center",flex:1},children:[y(vn,{size:8}),r<n-1&&y("div",{style:{flex:1,width:1,background:"var(--agd-stroke)"}})]},r))}),y("div",{style:{flex:1,display:"flex",flexDirection:"column",justifyContent:"space-around",paddingLeft:8},children:Array.from({length:n},(o,r)=>W("div",{style:{display:"flex",flexDirection:"column",gap:3},children:[y(A,{w:`${35+r*13%25}%`,h:3,strong:!0}),y(A,{w:`${50+r*17%30}%`,h:2})]},r))})]})}function Cx({width:e,height:t}){return W("div",{style:{height:"100%",borderRadius:8,border:"2px dashed var(--agd-stroke)",display:"flex",flexDirection:"column",alignItems:"center",justifyContent:"center",gap:t*.06},children:[W("svg",{width:"24",height:"24",viewBox:"0 0 24 24",fill:"none",children:[y("path",{d:"M12 16V4m0 0l-4 4m4-4l4 4",stroke:"var(--agd-stroke)",strokeWidth:"1.5"}),y("path",{d:"M4 17v2a1 1 0 001 1h14a1 1 0 001-1v-2",stroke:"var(--agd-stroke)",strokeWidth:"1.5"})]}),y(A,{w:e*.4,h:2}),y(A,{w:e*.25,h:2})]})}function Sx({width:e,height:t}){let n=Math.max(3,Math.min(8,Math.floor(t/20)));return W("div",{style:{height:"100%",borderRadius:6,background:"var(--agd-fill)",border:"1px solid var(--agd-stroke)",padding:8,display:"flex",flexDirection:"column",gap:4},children:[W("div",{style:{display:"flex",gap:3,marginBottom:4},children:[y(vn,{size:6}),y(vn,{size:6}),y(vn,{size:6})]}),Array.from({length:n},(o,r)=>y("div",{style:{display:"flex",gap:6,paddingLeft:r>0&&r<n-1?12:0},children:y(A,{w:`${25+r*23%50}%`,h:2,strong:r===0})},r))]})}function Mx({width:e,height:t}){let r=Math.min((e-16)/7,(t-40)/6);return W("div",{style:{height:"100%",display:"flex",flexDirection:"column"},children:[W("div",{style:{display:"flex",alignItems:"center",justifyContent:"space-between",padding:"6px 8px"},children:[y("span",{style:{fontSize:8,color:"var(--agd-stroke)"},children:"\u2039"}),y(A,{w:e*.3,h:3,strong:!0}),y("span",{style:{fontSize:8,color:"var(--agd-stroke)"},children:"\u203A"})]}),W("div",{style:{display:"grid",gridTemplateColumns:"repeat(7, 1fr)",gap:2,padding:"0 4px",flex:1},children:[Array.from({length:7},(i,l)=>y("div",{style:{display:"flex",alignItems:"center",justifyContent:"center",height:r*.6},children:y(A,{w:r*.5,h:2})},`h${l}`)),Array.from({length:35},(i,l)=>y("div",{style:{display:"flex",alignItems:"center",justifyContent:"center",height:r},children:y("div",{style:{width:r*.6,height:r*.6,borderRadius:"50%",background:l===12?"var(--agd-bar)":"transparent",display:"flex",alignItems:"center",justifyContent:"center"},children:y("div",{style:{width:2,height:2,borderRadius:1,background:"var(--agd-bar-strong)",opacity:l===12?1:.3}})})},l))]})]})}function Ex({width:e,height:t}){return W("div",{style:{height:"100%",borderRadius:8,border:"1px dashed var(--agd-stroke)",background:"var(--agd-fill)",display:"flex",alignItems:"center",padding:"0 10px",gap:8},children:[y(vn,{size:Math.min(32,t*.55)}),W("div",{style:{flex:1,display:"flex",flexDirection:"column",gap:3},children:[y(A,{w:"50%",h:3,strong:!0}),y(A,{w:"75%",h:2})]}),y(A,{w:30,h:2})]})}function Lx({width:e,height:t}){return W("div",{style:{height:"100%",display:"flex",flexDirection:"column"},children:[y("div",{style:{height:"50%",background:"var(--agd-fill)",borderBottom:"1px dashed var(--agd-stroke)"}}),W("div",{style:{flex:1,padding:10,display:"flex",flexDirection:"column",gap:5},children:[y(A,{w:"65%",h:4,strong:!0}),y(A,{w:"40%",h:3}),y("div",{style:{flex:1}}),W("div",{style:{display:"flex",alignItems:"center",justifyContent:"space-between"},children:[y(A,{w:"30%",h:5,strong:!0}),y(ut,{w:Math.min(70,e*.3),h:26,radius:4,style:{background:"var(--agd-bar)"}})]})]})]})}function Nx({width:e,height:t}){let n=Math.min(48,t*.3);return W("div",{style:{height:"100%",display:"flex",flexDirection:"column",alignItems:"center",justifyContent:"center",gap:t*.06},children:[y(vn,{size:n}),y(A,{w:e*.45,h:4,strong:!0}),y(A,{w:e*.3,h:2}),W("div",{style:{display:"flex",gap:e*.08,marginTop:t*.04},children:[W("div",{style:{display:"flex",flexDirection:"column",alignItems:"center",gap:2},children:[y(A,{w:20,h:3,strong:!0}),y(A,{w:28,h:2})]}),W("div",{style:{display:"flex",flexDirection:"column",alignItems:"center",gap:2},children:[y(A,{w:20,h:3,strong:!0}),y(A,{w:28,h:2})]}),W("div",{style:{display:"flex",flexDirection:"column",alignItems:"center",gap:2},children:[y(A,{w:20,h:3,strong:!0}),y(A,{w:28,h:2})]})]})]})}function Ix({width:e,height:t}){let n=Math.max(e*.6,80),o=Math.max(3,Math.floor(t/40));return W("div",{style:{height:"100%",display:"flex"},children:[y("div",{style:{width:e-n,background:"var(--agd-fill)",opacity:.3}}),W("div",{style:{flex:1,borderLeft:"1px solid var(--agd-stroke)",display:"flex",flexDirection:"column",padding:e*.04},children:[W("div",{style:{display:"flex",justifyContent:"space-between",alignItems:"center",marginBottom:t*.06},children:[y(A,{w:n*.4,h:4,strong:!0}),y("div",{style:{width:12,height:12,border:"1px solid var(--agd-stroke)",borderRadius:3}})]}),Array.from({length:o},(r,i)=>y("div",{style:{padding:"6px 0"},children:y(A,{w:`${50+i*17%35}%`,h:2,strong:i===0})},i))]})]})}function Rx({width:e,height:t}){return W("div",{style:{height:"100%",display:"flex",flexDirection:"column",alignItems:"center"},children:[W("div",{style:{flex:1,width:"100%",borderRadius:8,border:"1px dashed var(--agd-stroke)",background:"var(--agd-fill)",padding:10,display:"flex",flexDirection:"column",gap:5},children:[y(A,{w:"70%",h:3,strong:!0}),y(A,{w:"90%",h:2}),y(A,{w:"60%",h:2})]}),y("div",{style:{width:10,height:10,background:"var(--agd-fill)",border:"1px dashed var(--agd-stroke)",borderTop:"none",borderLeft:"none",transform:"rotate(45deg)",marginTop:-6}})]})}function $x({width:e,height:t}){let n=Math.min(t*.7,e*.3);return W("div",{style:{height:"100%",display:"flex",alignItems:"center",gap:e*.08},children:[y(ut,{w:n,h:n,radius:n*.25}),y(A,{w:e*.45,h:Math.max(4,t*.2),strong:!0})]})}function Tx({width:e,height:t}){let n=Math.max(2,Math.min(5,Math.floor(t/56)));return y("div",{style:{display:"flex",flexDirection:"column",height:"100%"},children:Array.from({length:n},(o,r)=>W("div",{style:{borderBottom:"1px solid var(--agd-stroke)",padding:"8px 6px",display:"flex",alignItems:"center",justifyContent:"space-between",flex:r===0?2:1},children:[W("div",{style:{display:"flex",alignItems:"center",gap:6},children:[y("span",{style:{fontSize:9,fontWeight:700,color:"var(--agd-stroke)"},children:"Q"}),y(A,{w:e*(.3+r*13%25/100),h:3,strong:!0})]}),y("span",{style:{fontSize:8,color:"var(--agd-stroke)"},children:r===0?"\u25BC":"\u25B6"})]},r))})}function Px({width:e,height:t}){let n=Math.max(2,Math.min(4,Math.floor(e/120))),o=Math.max(1,Math.min(3,Math.floor(t/120)));return y("div",{style:{display:"grid",gridTemplateColumns:`repeat(${n}, 1fr)`,gridTemplateRows:`repeat(${o}, 1fr)`,gap:4,height:"100%"},children:Array.from({length:n*o},(r,i)=>y("div",{style:{borderRadius:4,border:"1px dashed var(--agd-stroke)",background:"var(--agd-fill)",position:"relative",overflow:"hidden"},children:W("svg",{width:"100%",height:"100%",viewBox:"0 0 100 100",preserveAspectRatio:"none",fill:"none",children:[y("line",{x1:"0",y1:"0",x2:"100",y2:"100",stroke:"var(--agd-stroke)",strokeWidth:"0.5"}),y("line",{x1:"100",y1:"0",x2:"0",y2:"100",stroke:"var(--agd-stroke)",strokeWidth:"0.5"})]})},i))})}function Dx({width:e,height:t}){let n=Math.min(e,t);return W("svg",{width:"100%",height:"100%",viewBox:`0 0 ${e} ${t}`,fill:"none",children:[y("rect",{x:"1",y:(t-n+2)/2,width:n-2,height:n-2,rx:n*.15,stroke:"var(--agd-stroke)",strokeWidth:"1.5"}),y("path",{d:`M${n*.25} ${t/2}l${n*.2} ${n*.2} ${n*.3}-${n*.35}`,stroke:"var(--agd-bar)",strokeWidth:"1.5",fill:"none",strokeLinecap:"round",strokeLinejoin:"round"})]})}function Bx({width:e,height:t}){let n=Math.min(e,t)/2-1;return W("svg",{width:"100%",height:"100%",viewBox:`0 0 ${e} ${t}`,fill:"none",children:[y("circle",{cx:e/2,cy:t/2,r:n,stroke:"var(--agd-stroke)",strokeWidth:"1.5"}),y("circle",{cx:e/2,cy:t/2,r:n*.45,fill:"var(--agd-bar)"})]})}function zx({width:e,height:t}){let n=Math.max(2,t*.12),o=Math.min(t*.35,10),r=e*.55;return W("div",{style:{height:"100%",display:"flex",alignItems:"center",position:"relative"},children:[y("div",{style:{width:"100%",height:n,borderRadius:n/2,background:"var(--agd-fill)",border:"1px solid var(--agd-stroke)",position:"relative"},children:y("div",{style:{width:r,height:"100%",borderRadius:n/2,background:"var(--agd-bar)"}})}),y("div",{style:{position:"absolute",left:r-o,width:o*2,height:o*2,borderRadius:"50%",border:"1.5px solid var(--agd-stroke)",background:"var(--agd-fill)"}})]})}function Ox({width:e,height:t}){let n=Math.min(36,t*.15),o=7,r=4,i=Math.min((e-16)/o,(t-n-40)/(r+1));return W("div",{style:{height:"100%",display:"flex",flexDirection:"column",gap:4},children:[W("div",{style:{height:n,borderRadius:4,border:"1px dashed var(--agd-stroke)",background:"var(--agd-fill)",display:"flex",alignItems:"center",padding:"0 8px",justifyContent:"space-between"},children:[y(A,{w:"40%",h:2}),W("svg",{width:"12",height:"12",viewBox:"0 0 16 16",fill:"none",children:[y("rect",{x:"2",y:"3",width:"12",height:"11",rx:"1",stroke:"var(--agd-stroke)",strokeWidth:"1"}),y("line",{x1:"2",y1:"6",x2:"14",y2:"6",stroke:"var(--agd-stroke)",strokeWidth:"0.5"})]})]}),W("div",{style:{flex:1,borderRadius:6,border:"1px dashed var(--agd-stroke)",background:"var(--agd-fill)",display:"flex",flexDirection:"column"},children:[W("div",{style:{display:"flex",alignItems:"center",justifyContent:"space-between",padding:"4px 6px"},children:[y("span",{style:{fontSize:7,color:"var(--agd-stroke)"},children:"\u2039"}),y(A,{w:e*.25,h:2,strong:!0}),y("span",{style:{fontSize:7,color:"var(--agd-stroke)"},children:"\u203A"})]}),y("div",{style:{display:"grid",gridTemplateColumns:`repeat(${o}, 1fr)`,gap:1,padding:"0 4px",flex:1},children:Array.from({length:o*r},(l,s)=>y("div",{style:{display:"flex",alignItems:"center",justifyContent:"center",height:i},children:y("div",{style:{width:i*.5,height:i*.5,borderRadius:"50%",background:s===10?"var(--agd-bar)":"transparent"},children:y("div",{style:{width:"100%",height:"100%",display:"flex",alignItems:"center",justifyContent:"center"},children:y("div",{style:{width:1.5,height:1.5,borderRadius:1,background:"var(--agd-bar-strong)",opacity:s===10?1:.25}})})})},s))})]})]})}function Ax({width:e,height:t}){return W("div",{style:{height:"100%",display:"flex",flexDirection:"column",gap:t*.08,padding:4},children:[y("div",{style:{width:"100%",height:t*.2,borderRadius:4,background:"var(--agd-fill)"}}),y("div",{style:{width:"70%",height:Math.max(6,t*.1),borderRadius:3,background:"var(--agd-fill)"}}),y("div",{style:{width:"90%",height:Math.max(4,t*.06),borderRadius:3,background:"var(--agd-fill)"}}),y("div",{style:{width:"50%",height:Math.max(4,t*.06),borderRadius:3,background:"var(--agd-fill)"}})]})}function Fx({width:e,height:t}){return y("div",{style:{height:"100%",display:"flex",alignItems:"center",gap:6},children:W("div",{style:{height:"100%",flex:1,borderRadius:t/2,border:"1px solid var(--agd-stroke)",background:"var(--agd-fill)",display:"flex",alignItems:"center",padding:`0 ${t*.3}px`,gap:4},children:[y(A,{w:"60%",h:2,strong:!0}),y("div",{style:{width:Math.max(6,t*.3),height:Math.max(6,t*.3),borderRadius:"50%",border:"1px solid var(--agd-stroke)",flexShrink:0,marginLeft:"auto"}})]})})}function Wx({width:e,height:t}){let n=Math.min(e,t);return y("svg",{width:"100%",height:"100%",viewBox:`0 0 ${e} ${t}`,fill:"none",children:y("path",{d:`M${e/2} ${(t-n)/2+n*.1}l${n*.12} ${n*.25} ${n*.28} ${n*.04}-${n*.2} ${n*.2} ${n*.05} ${n*.28}-${n*.25}-${n*.12}-${n*.25} ${n*.12} ${n*.05}-${n*.28}-${n*.2}-${n*.2} ${n*.28}-${n*.04}z`,stroke:"var(--agd-stroke)",strokeWidth:"1",fill:"var(--agd-fill)"})})}function jx({width:e,height:t}){let n=Math.min(e,t)/2-2;return W("svg",{width:"100%",height:"100%",viewBox:`0 0 ${e} ${t}`,fill:"none",children:[y("circle",{cx:e/2,cy:t/2,r:n,stroke:"var(--agd-stroke)",strokeWidth:"1.5",opacity:".2"}),y("path",{d:`M${e/2} ${t/2-n}a${n} ${n} 0 0 1 ${n} ${n}`,stroke:"var(--agd-bar-strong)",strokeWidth:"1.5",strokeLinecap:"round"})]})}function Hx({width:e,height:t}){let n=Math.min(36,t*.25,e*.12),o=Math.max(1,Math.min(3,Math.floor(t/80)));return y("div",{style:{display:"flex",flexDirection:"column",height:"100%",justifyContent:"space-around",padding:8},children:Array.from({length:o},(r,i)=>W("div",{style:{display:"flex",gap:e*.04,alignItems:"flex-start"},children:[y(ut,{w:n,h:n,radius:n*.25}),W("div",{style:{flex:1,display:"flex",flexDirection:"column",gap:4},children:[y(A,{w:`${40+i*13%20}%`,h:3,strong:!0}),y(A,{w:`${60+i*17%25}%`,h:2})]})]},i))})}function Ux({width:e,height:t}){let n=Math.max(2,Math.min(4,Math.floor(e/120))),o=Math.min(36,t*.25);return W("div",{style:{height:"100%",display:"flex",flexDirection:"column",alignItems:"center",gap:t*.06,padding:t*.06},children:[y(A,{w:e*.3,h:4,strong:!0}),y("div",{style:{display:"flex",gap:e*.06,justifyContent:"center",flex:1,alignItems:"center"},children:Array.from({length:n},(r,i)=>W("div",{style:{display:"flex",flexDirection:"column",alignItems:"center",gap:6},children:[y(vn,{size:o}),y(A,{w:e*.12,h:3,strong:!0}),y(A,{w:e*.08,h:2})]},i))})]})}function Yx({width:e,height:t}){let n=Math.max(2,Math.min(3,Math.floor(t/80)));return W("div",{style:{height:"100%",display:"flex",flexDirection:"column",alignItems:"center",padding:e*.06,gap:t*.04},children:[y(A,{w:e*.5,h:Math.max(5,t*.04),strong:!0}),y(A,{w:e*.35,h:2}),y("div",{style:{width:"100%",display:"flex",flexDirection:"column",gap:t*.03,marginTop:t*.04},children:Array.from({length:n},(o,r)=>W("div",{style:{display:"flex",flexDirection:"column",gap:3},children:[y(A,{w:Math.min(60,e*.2),h:2}),y(ut,{w:"100%",h:Math.min(32,t*.1),radius:4})]},r))}),y(ut,{w:"100%",h:Math.min(36,t*.12),radius:6,style:{marginTop:t*.03,background:"var(--agd-bar)"}}),y(A,{w:e*.4,h:2})]})}function Qx({width:e,height:t}){return W("div",{style:{height:"100%",display:"flex",flexDirection:"column",padding:e*.04,gap:t*.03},children:[y(A,{w:e*.4,h:4,strong:!0}),y(A,{w:e*.6,h:2}),W("div",{style:{display:"flex",gap:6,marginTop:t*.03},children:[W("div",{style:{flex:1,display:"flex",flexDirection:"column",gap:3},children:[y(A,{w:50,h:2}),y(ut,{w:"100%",h:Math.min(28,t*.1),radius:4})]}),W("div",{style:{flex:1,display:"flex",flexDirection:"column",gap:3},children:[y(A,{w:40,h:2}),y(ut,{w:"100%",h:Math.min(28,t*.1),radius:4})]})]}),W("div",{style:{display:"flex",flexDirection:"column",gap:3},children:[y(A,{w:50,h:2}),y(ut,{w:"100%",h:Math.min(28,t*.1),radius:4})]}),W("div",{style:{display:"flex",flexDirection:"column",gap:3,flex:1},children:[y(A,{w:60,h:2}),y(ut,{w:"100%",h:"100%",radius:4})]}),y(ut,{w:Math.min(120,e*.3),h:Math.min(30,t*.1),radius:6,style:{alignSelf:"flex-end",background:"var(--agd-bar)"}})]})}var Vx={navigation:D2,hero:B2,sidebar:z2,footer:O2,modal:A2,card:F2,text:W2,image:j2,table:H2,list:U2,button:Y2,input:Q2,form:V2,tabs:X2,avatar:G2,badge:q2,header:K2,section:J2,grid:Z2,dropdown:ex,toggle:tx,search:nx,toast:ox,progress:rx,chart:ix,video:lx,tooltip:sx,breadcrumb:ax,pagination:cx,divider:dx,accordion:ux,carousel:_x,pricing:fx,testimonial:hx,cta:px,alert:mx,banner:gx,stat:yx,stepper:xx,tag:vx,rating:wx,map:bx,timeline:kx,fileUpload:Cx,codeBlock:Sx,calendar:Mx,notification:Ex,productCard:Lx,profile:Nx,drawer:Ix,popover:Rx,logo:$x,faq:Tx,gallery:Px,checkbox:Dx,radio:Bx,slider:zx,datePicker:Ox,skeleton:Ax,chip:Fx,icon:Wx,spinner:jx,feature:Hx,team:Ux,login:Yx,contact:Qx};function Xx({type:e,width:t,height:n,text:o}){let r=Vx[e];return r?y("div",{style:{width:"100%",height:"100%",padding:8,position:"relative",pointerEvents:"none"},children:y(r,{width:t,height:n,text:o})}):y("div",{style:{width:"100%",height:"100%",display:"flex",alignItems:"center",justifyContent:"center"},children:y("span",{style:{fontSize:10,fontWeight:600,color:"var(--agd-text-3)",textTransform:"uppercase",letterSpacing:"0.06em",opacity:.5},children:e})})}var Gx=`.styles-module__overlay___aWh-q svg[fill=none],
.styles-module__rearrangeOverlay___-3R3t svg[fill=none] {
  fill: none !important;
}
.styles-module__overlay___aWh-q svg[fill=none] :not([fill]),
.styles-module__rearrangeOverlay___-3R3t svg[fill=none] :not([fill]) {
  fill: none !important;
}

.styles-module__overlayExiting___iEmYr {
  opacity: 0 !important;
  transition: opacity 0.25s ease !important;
  pointer-events: none !important;
}

.styles-module__overlay___aWh-q {
  position: fixed;
  inset: 0;
  z-index: 99995;
  pointer-events: auto;
  cursor: default;
  animation: styles-module__overlayFadeIn___aECVy 0.15s ease;
  --agd-stroke: rgba(59, 130, 246, 0.35);
  --agd-fill: rgba(59, 130, 246, 0.06);
  --agd-bar: rgba(59, 130, 246, 0.18);
  --agd-bar-strong: rgba(59, 130, 246, 0.28);
  --agd-text-3: rgba(255, 255, 255, 0.6);
  --agd-surface: #fff;
}
.styles-module__overlay___aWh-q.styles-module__light___ORIft {
  --agd-surface: #fff;
}
.styles-module__overlay___aWh-q:not(.styles-module__light___ORIft) {
  --agd-surface: #141414;
}
.styles-module__overlay___aWh-q.styles-module__wireframe___itvQU {
  --agd-stroke: rgba(249, 115, 22, 0.35);
  --agd-fill: rgba(249, 115, 22, 0.06);
  --agd-bar: rgba(249, 115, 22, 0.18);
  --agd-bar-strong: rgba(249, 115, 22, 0.28);
}
.styles-module__overlay___aWh-q.styles-module__placing___45yD8 {
  cursor: crosshair;
}
.styles-module__overlay___aWh-q.styles-module__passthrough___xaFeE {
  pointer-events: none;
}

.styles-module__blankCanvas___t2Eue {
  position: fixed;
  inset: 0;
  z-index: 99994;
  background: #fff;
  opacity: 0;
  pointer-events: none;
  transition: opacity 0.25s ease;
}
.styles-module__blankCanvas___t2Eue.styles-module__visible___OKKqX {
  opacity: var(--canvas-opacity, 1);
  pointer-events: auto;
}
.styles-module__blankCanvas___t2Eue::after {
  content: "";
  position: absolute;
  inset: 0;
  background-image: radial-gradient(circle, rgba(0, 0, 0, 0.08) 1px, transparent 1px);
  background-size: 24px 24px;
  background-position: 12px 12px;
  pointer-events: none;
  transition: opacity 0.2s ease;
}
.styles-module__blankCanvas___t2Eue.styles-module__gridActive___OZ-cf::after {
  opacity: 1;
  background-image: radial-gradient(circle, rgba(0, 0, 0, 0.22) 1px, transparent 1px);
}

.styles-module__paletteHeader___-Q5gQ {
  padding: 0 1rem 0.375rem;
}

.styles-module__paletteHeaderTitle___oHqZC {
  font-size: 0.8125rem;
  font-weight: 500;
  color: #fff;
  letter-spacing: -0.0094em;
}
.styles-module__light___ORIft .styles-module__paletteHeaderTitle___oHqZC {
  color: rgba(0, 0, 0, 0.85);
}

.styles-module__paletteHeaderDesc___6i74T {
  font-size: 0.6875rem;
  font-weight: 300;
  color: rgba(255, 255, 255, 0.45);
  margin-top: 2px;
  line-height: 14px;
}
.styles-module__light___ORIft .styles-module__paletteHeaderDesc___6i74T {
  color: rgba(0, 0, 0, 0.45);
}
.styles-module__paletteHeaderDesc___6i74T a {
  color: rgba(255, 255, 255, 0.8);
  text-decoration: underline dotted;
  text-decoration-color: rgba(255, 255, 255, 0.2);
  text-underline-offset: 2px;
  transition: color 0.15s ease;
}
.styles-module__paletteHeaderDesc___6i74T a:hover {
  color: #fff;
}
.styles-module__light___ORIft .styles-module__paletteHeaderDesc___6i74T a {
  color: rgba(0, 0, 0, 0.6);
  text-decoration-color: rgba(0, 0, 0, 0.2);
}
.styles-module__light___ORIft .styles-module__paletteHeaderDesc___6i74T a:hover {
  color: rgba(0, 0, 0, 0.85);
}

.styles-module__wireframePurposeWrap___To-tS {
  display: grid;
  grid-template-rows: 1fr;
  transition: grid-template-rows 0.2s ease, opacity 0.15s ease;
  opacity: 1;
}
.styles-module__wireframePurposeWrap___To-tS.styles-module__collapsed___Ms9vS {
  grid-template-rows: 0fr;
  opacity: 0;
}

.styles-module__wireframePurposeInner___Lrahs {
  overflow: hidden;
}

.styles-module__wireframePurposeInput___7EtBN {
  display: block;
  width: calc(100% - 2rem);
  margin: 0.25rem 1rem 0.375rem;
  padding: 0.375rem 0.5rem;
  font-size: 0.8125rem;
  font-family: inherit;
  color: rgba(255, 255, 255, 0.85);
  background: rgba(255, 255, 255, 0.03);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 0.375rem;
  resize: none;
  outline: none;
  transition: border-color 0.15s ease;
  letter-spacing: -0.0094em;
}
.styles-module__wireframePurposeInput___7EtBN::placeholder {
  color: rgba(255, 255, 255, 0.3);
}
.styles-module__wireframePurposeInput___7EtBN:focus {
  border-color: rgba(255, 255, 255, 0.3);
  background: rgba(255, 255, 255, 0.05);
}
.styles-module__light___ORIft .styles-module__wireframePurposeInput___7EtBN {
  color: rgba(0, 0, 0, 0.7);
  background: rgba(0, 0, 0, 0.03);
  border-color: rgba(0, 0, 0, 0.1);
}
.styles-module__light___ORIft .styles-module__wireframePurposeInput___7EtBN::placeholder {
  color: rgba(0, 0, 0, 0.3);
}
.styles-module__light___ORIft .styles-module__wireframePurposeInput___7EtBN:focus {
  border-color: rgba(0, 0, 0, 0.25);
  background: rgba(0, 0, 0, 0.05);
}

.styles-module__canvasToggle___-QqSy {
  width: calc(100% - 2rem);
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.375rem;
  margin: 0.25rem 1rem 0.25rem;
  padding: 0.375rem 0.5rem;
  border-radius: 0.5rem;
  cursor: pointer;
  border: 1px dashed rgba(255, 255, 255, 0.1);
  background: transparent;
  transition: background 0.15s ease, border-color 0.15s ease;
}
.styles-module__canvasToggle___-QqSy:hover {
  background: rgba(255, 255, 255, 0.04);
  border-color: rgba(255, 255, 255, 0.15);
}
.styles-module__canvasToggle___-QqSy.styles-module__active___hosp7 {
  background: #f97316;
  border-color: transparent;
  border-style: solid;
  box-shadow: none;
}
.styles-module__light___ORIft .styles-module__canvasToggle___-QqSy {
  border-color: rgba(0, 0, 0, 0.08);
}
.styles-module__light___ORIft .styles-module__canvasToggle___-QqSy:hover {
  background: rgba(0, 0, 0, 0.02);
  border-color: rgba(0, 0, 0, 0.12);
}
.styles-module__light___ORIft .styles-module__canvasToggle___-QqSy.styles-module__active___hosp7 {
  background: #f97316;
  border-color: transparent;
  border-style: solid;
  box-shadow: none;
}

.styles-module__canvasToggleIcon___7pJ82 {
  width: 14px;
  height: 14px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  color: rgba(255, 255, 255, 0.35);
}
.styles-module__active___hosp7 .styles-module__canvasToggleIcon___7pJ82 {
  color: rgba(255, 255, 255, 0.85);
}
.styles-module__light___ORIft .styles-module__canvasToggleIcon___7pJ82 {
  color: rgba(0, 0, 0, 0.25);
}
.styles-module__light___ORIft .styles-module__active___hosp7 .styles-module__canvasToggleIcon___7pJ82 {
  color: rgba(255, 255, 255, 0.85);
}

.styles-module__canvasToggleLabel___OanpY {
  font-size: 0.8125rem;
  font-weight: 400;
  color: rgba(255, 255, 255, 0.6);
  letter-spacing: -0.0094em;
}
.styles-module__active___hosp7 .styles-module__canvasToggleLabel___OanpY {
  color: #fff;
}
.styles-module__light___ORIft .styles-module__canvasToggleLabel___OanpY {
  color: rgba(0, 0, 0, 0.5);
}
.styles-module__light___ORIft .styles-module__active___hosp7 .styles-module__canvasToggleLabel___OanpY {
  color: #fff;
}

.styles-module__placement___zcxv8 {
  position: absolute;
  border: 1.5px dashed rgba(59, 130, 246, 0.4);
  border-radius: 6px;
  background: rgba(59, 130, 246, 0.08);
  cursor: grab;
  transition: box-shadow 0.15s, border-color 0.15s, opacity 0.15s ease, transform 0.15s ease;
  -webkit-user-select: none;
  user-select: none;
  pointer-events: auto;
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.08);
  animation: styles-module__placementEnter___TdRhf 0.25s cubic-bezier(0.34, 1.2, 0.64, 1);
}
.styles-module__placement___zcxv8:active {
  cursor: grabbing;
}
.styles-module__placement___zcxv8:hover {
  border-color: rgba(59, 130, 246, 0.5);
  background: rgba(59, 130, 246, 0.1);
  box-shadow: 0 2px 8px rgba(59, 130, 246, 0.12);
}
.styles-module__placement___zcxv8.styles-module__selected___6yrp6 {
  border-color: #3c82f7;
  border-style: solid;
  background: rgba(59, 130, 246, 0.1);
  box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.15), 0 2px 8px rgba(59, 130, 246, 0.15);
}
.styles-module__placement___zcxv8.styles-module__selected___6yrp6:hover {
  box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.15), 0 2px 8px rgba(59, 130, 246, 0.15);
}
.styles-module__wireframe___itvQU .styles-module__placement___zcxv8 {
  border-color: rgba(249, 115, 22, 0.4);
  background: rgba(249, 115, 22, 0.08);
}
.styles-module__wireframe___itvQU .styles-module__placement___zcxv8:hover {
  border-color: rgba(249, 115, 22, 0.5);
  background: rgba(249, 115, 22, 0.1);
  box-shadow: 0 2px 8px rgba(249, 115, 22, 0.12);
}
.styles-module__wireframe___itvQU .styles-module__placement___zcxv8.styles-module__selected___6yrp6 {
  border-color: #f97316;
  background: rgba(249, 115, 22, 0.1);
  box-shadow: 0 0 0 2px rgba(249, 115, 22, 0.15), 0 2px 8px rgba(249, 115, 22, 0.15);
}
.styles-module__wireframe___itvQU .styles-module__placement___zcxv8.styles-module__selected___6yrp6:hover {
  box-shadow: 0 0 0 2px rgba(249, 115, 22, 0.15), 0 2px 8px rgba(249, 115, 22, 0.15);
}
.styles-module__placement___zcxv8.styles-module__dragging___le6KZ {
  opacity: 0.85;
  z-index: 50;
}
.styles-module__placement___zcxv8.styles-module__exiting___YrM8F {
  opacity: 0;
  transform: scale(0.97);
  pointer-events: none;
  animation: none;
  transition: opacity 0.2s ease, transform 0.2s cubic-bezier(0.32, 0.72, 0, 1);
}

.styles-module__placementContent___f64A4 {
  width: 100%;
  height: 100%;
  overflow: hidden;
  pointer-events: none;
}

.styles-module__placementLabel___0KvWl {
  position: absolute;
  top: -18px;
  left: 0;
  font-size: 10px;
  font-weight: 600;
  color: rgba(59, 130, 246, 0.7);
  white-space: nowrap;
  pointer-events: none;
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
  text-shadow: 0 0 4px rgba(255, 255, 255, 0.8), 0 0 8px rgba(255, 255, 255, 0.5);
}
.styles-module__selected___6yrp6 .styles-module__placementLabel___0KvWl {
  color: #3c82f7;
}
.styles-module__wireframe___itvQU .styles-module__placementLabel___0KvWl {
  color: rgba(249, 115, 22, 0.7);
}
.styles-module__wireframe___itvQU .styles-module__selected___6yrp6 .styles-module__placementLabel___0KvWl {
  color: #f97316;
}

.styles-module__placementAnnotation___78pTr {
  position: absolute;
  bottom: -18px;
  left: 0;
  right: 0;
  font-weight: 450;
  color: rgba(0, 0, 0, 0.5);
  font-size: 10px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  pointer-events: none;
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
  text-shadow: 0 0 4px rgba(255, 255, 255, 0.9), 0 0 8px rgba(255, 255, 255, 0.6);
  opacity: 0;
  transform: translateY(-2px);
  transition: opacity 0.2s ease, transform 0.2s ease;
}
.styles-module__placementAnnotation___78pTr.styles-module__annotationVisible___mrUyA {
  opacity: 1;
  transform: translateY(0);
}

.styles-module__sectionAnnotation___aUIs0 {
  position: absolute;
  bottom: -18px;
  left: 0;
  right: 0;
  font-weight: 450;
  color: rgba(59, 130, 246, 0.6);
  font-size: 10px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  pointer-events: none;
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
  text-shadow: 0 0 4px rgba(255, 255, 255, 0.9), 0 0 8px rgba(255, 255, 255, 0.6);
  opacity: 0;
  transform: translateY(-2px);
  transition: opacity 0.2s ease, transform 0.2s ease;
}
.styles-module__sectionAnnotation___aUIs0.styles-module__annotationVisible___mrUyA {
  opacity: 1;
  transform: translateY(0);
}

.styles-module__handle___Ikbxm {
  position: absolute;
  width: 8px;
  height: 8px;
  background: #fff;
  border: 1.5px solid #3c82f7;
  border-radius: 2px;
  z-index: 12;
  box-shadow: 0 0 0 0.5px rgba(0, 0, 0, 0.1), 0 1px 2px rgba(0, 0, 0, 0.12);
  opacity: 0;
  transform: scale(0.3);
  pointer-events: none;
  will-change: opacity, transform;
  transition: opacity 0.2s ease-out, transform 0.25s cubic-bezier(0.34, 1.56, 0.64, 1);
}
.styles-module__placement___zcxv8:hover .styles-module__handle___Ikbxm, .styles-module__sectionOutline___s0hy-:hover .styles-module__handle___Ikbxm, .styles-module__ghostOutline___po-kO:hover .styles-module__handle___Ikbxm, .styles-module__placement___zcxv8:active .styles-module__handle___Ikbxm, .styles-module__sectionOutline___s0hy-:active .styles-module__handle___Ikbxm, .styles-module__ghostOutline___po-kO:active .styles-module__handle___Ikbxm, .styles-module__selected___6yrp6 .styles-module__handle___Ikbxm {
  opacity: 1;
  transform: scale(1);
  pointer-events: auto;
}
.styles-module__sectionOutline___s0hy- .styles-module__handle___Ikbxm {
  border-color: inherit;
}
.styles-module__wireframe___itvQU .styles-module__handle___Ikbxm {
  border-color: #f97316;
}

.styles-module__handleNw___4TMIj {
  top: -4px;
  left: -4px;
  cursor: nw-resize;
}

.styles-module__handleNe___mnsTh {
  top: -4px;
  right: -4px;
  cursor: ne-resize;
}

.styles-module__handleSe___oSFnk {
  bottom: -4px;
  right: -4px;
  cursor: se-resize;
}

.styles-module__handleSw___pi--Z {
  bottom: -4px;
  left: -4px;
  cursor: sw-resize;
}

.styles-module__handleN___aBA-Q,
.styles-module__handleE___0hM5u,
.styles-module__handleS___JjDRv,
.styles-module__handleW___ERWGQ {
  opacity: 0 !important;
  pointer-events: none !important;
}

.styles-module__edgeHandle___XxXdT {
  position: absolute;
  z-index: 11;
  display: flex;
  align-items: center;
  justify-content: center;
}
.styles-module__edgeHandle___XxXdT::after {
  content: "";
  position: absolute;
  border-radius: 4px;
  background: #3c82f7;
}
.styles-module__wireframe___itvQU .styles-module__edgeHandle___XxXdT::after {
  background: #f97316;
}
.styles-module__edgeHandle___XxXdT::after {
  opacity: 0;
  transition: opacity 0.1s ease, transform 0.1s ease;
  transform: scale(0.8);
}
.styles-module__edgeHandle___XxXdT:hover::after {
  opacity: 0.85;
  transform: scale(1);
}
.styles-module__edgeHandle___XxXdT svg {
  position: relative;
  z-index: 1;
  opacity: 0;
  transition: opacity 0.1s ease;
  filter: drop-shadow(0 0 2px var(--agd-surface));
}
.styles-module__edgeHandle___XxXdT:hover svg {
  opacity: 1;
}

.styles-module__edgeN___-JJDj,
.styles-module__edgeS___66lMX {
  left: 12px;
  right: 12px;
  height: 12px;
  cursor: n-resize;
}
.styles-module__edgeN___-JJDj::after,
.styles-module__edgeS___66lMX::after {
  width: 24px;
  height: 4px;
}

.styles-module__edgeN___-JJDj {
  top: -6px;
}

.styles-module__edgeS___66lMX {
  bottom: -6px;
  cursor: s-resize;
}

.styles-module__edgeE___1bGDa,
.styles-module__edgeW___lHQNo {
  top: 12px;
  bottom: 12px;
  width: 12px;
  cursor: e-resize;
}
.styles-module__edgeE___1bGDa::after,
.styles-module__edgeW___lHQNo::after {
  width: 4px;
  height: 24px;
}

.styles-module__edgeE___1bGDa {
  right: -6px;
}

.styles-module__edgeW___lHQNo {
  left: -6px;
  cursor: w-resize;
}

.styles-module__deleteButton___LkGCb {
  position: absolute;
  top: -8px;
  right: -8px;
  width: 18px;
  height: 18px;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.9);
  backdrop-filter: blur(8px);
  border: 1px solid rgba(0, 0, 0, 0.08);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
  color: rgba(0, 0, 0, 0.35);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 10px;
  line-height: 1;
  z-index: 15;
  pointer-events: none;
  opacity: 0;
  transform: scale(0.8);
  will-change: opacity, transform;
  transition: opacity 0.2s ease-out, transform 0.2s cubic-bezier(0.34, 1.56, 0.64, 1), background 0.12s ease, color 0.12s ease, border-color 0.12s ease, box-shadow 0.12s ease;
}
.styles-module__placement___zcxv8:hover .styles-module__deleteButton___LkGCb, .styles-module__selected___6yrp6 .styles-module__deleteButton___LkGCb, .styles-module__sectionOutline___s0hy-:hover .styles-module__deleteButton___LkGCb, .styles-module__sectionOutline___s0hy-.styles-module__selected___6yrp6 .styles-module__deleteButton___LkGCb, .styles-module__ghostOutline___po-kO:hover .styles-module__deleteButton___LkGCb, .styles-module__ghostOutline___po-kO.styles-module__selected___6yrp6 .styles-module__deleteButton___LkGCb {
  opacity: 1;
  transform: scale(1);
  pointer-events: auto;
}
.styles-module__deleteButton___LkGCb:hover {
  background: #ef4444;
  color: #fff;
  border-color: #ef4444;
  box-shadow: 0 1px 4px rgba(239, 68, 68, 0.3);
  transform: scale(1.1);
}
.styles-module__overlay___aWh-q:not(.styles-module__light___ORIft) .styles-module__deleteButton___LkGCb, .styles-module__rearrangeOverlay___-3R3t:not(.styles-module__light___ORIft) .styles-module__deleteButton___LkGCb {
  background: rgba(40, 40, 40, 0.9);
  border-color: rgba(255, 255, 255, 0.1);
  color: rgba(255, 255, 255, 0.5);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.25);
}
.styles-module__overlay___aWh-q:not(.styles-module__light___ORIft) .styles-module__deleteButton___LkGCb:hover, .styles-module__rearrangeOverlay___-3R3t:not(.styles-module__light___ORIft) .styles-module__deleteButton___LkGCb:hover {
  background: #ef4444;
  color: #fff;
  border-color: #ef4444;
}

.styles-module__drawBox___BrVAa {
  position: fixed;
  pointer-events: none;
  z-index: 99996;
  border: 2px solid #3c82f7;
  border-radius: 6px;
  background: rgba(59, 130, 246, 0.15);
}

.styles-module__selectBox___Iu8kB {
  position: fixed;
  pointer-events: none;
  z-index: 99996;
  border: 1px dashed #3c82f7;
  background: rgba(59, 130, 246, 0.08);
  border-radius: 2px;
}

.styles-module__sizeIndicator___7zJ4y {
  position: fixed;
  pointer-events: none;
  z-index: 100001;
  font-size: 10px;
  color: #fff;
  background: #3c82f7;
  padding: 2px 6px;
  border-radius: 4px;
  white-space: nowrap;
  font-weight: 500;
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.2);
}

.styles-module__guideLine___DUQY2 {
  pointer-events: none;
  z-index: 100001;
  background: #f0f;
  opacity: 0.5;
}

.styles-module__dragPreview___onPbU {
  position: fixed;
  z-index: 100002;
  pointer-events: none;
  border: 1.5px dashed #3c82f7;
  border-radius: 6px;
  background: rgba(59, 130, 246, 0.1);
  backdrop-filter: blur(8px);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 9px;
  font-weight: 600;
  color: #3c82f7;
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  box-shadow: 0 4px 16px rgba(59, 130, 246, 0.15);
  transition: width 0.08s ease, height 0.08s ease, opacity 0.08s ease;
}

.styles-module__dragPreviewWireframe___jsg0G {
  border-color: #f97316;
  background: rgba(249, 115, 22, 0.1);
  color: #f97316;
  box-shadow: 0 4px 16px rgba(249, 115, 22, 0.15);
}

.styles-module__palette___C7iSH {
  position: absolute;
  right: 5px;
  bottom: calc(100% + 0.5rem);
  width: 256px;
  overflow: hidden;
  background: #1c1c1c;
  border: none;
  border-radius: 1rem;
  padding: 13px 0 16px;
  box-shadow: 0 1px 8px rgba(0, 0, 0, 0.25), 0 0 0 1px rgba(0, 0, 0, 0.04);
  z-index: 100001;
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
  cursor: default;
  opacity: 0;
  filter: blur(5px);
}
.styles-module__palette___C7iSH .styles-module__paletteItem___6TlnA,
.styles-module__palette___C7iSH .styles-module__paletteItemLabel___6ncO4,
.styles-module__palette___C7iSH .styles-module__paletteSectionTitle___PqnjX,
.styles-module__palette___C7iSH .styles-module__paletteFooter___QYnAG {
  transition: background 0.25s ease, color 0.25s ease, border-color 0.25s ease;
}
.styles-module__palette___C7iSH {
  opacity: 0;
  transform: translateY(var(--panel-offset-y, 4px)) scale(0.98);
  transform-origin: var(--panel-origin, bottom right);
  filter: blur(2px);
  pointer-events: none;
  visibility: hidden;
  transition: opacity 120ms cubic-bezier(0.25, 0.46, 0.45, 0.94), transform 120ms cubic-bezier(0.25, 0.46, 0.45, 0.94), filter 120ms cubic-bezier(0.25, 0.46, 0.45, 0.94);
}
.styles-module__palette___C7iSH[data-panel-present=true] {
  visibility: visible;
}
.styles-module__palette___C7iSH[data-panel-open=true] {
  opacity: 1;
  transform: translateY(0) scale(1);
  filter: blur(0);
  pointer-events: auto;
  transition-duration: 160ms;
}
@media (prefers-reduced-motion: reduce) {
  .styles-module__palette___C7iSH {
    transition: none;
    transform: none;
    filter: none;
  }
}
.styles-module__palette___C7iSH.styles-module__light___ORIft {
  background: #fff;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08), 0 4px 16px rgba(0, 0, 0, 0.06), 0 0 0 1px rgba(0, 0, 0, 0.04);
}

.styles-module__paletteSection___V8DEA {
  padding: 0 1rem;
}
.styles-module__paletteSection___V8DEA + .styles-module__paletteSection___V8DEA {
  margin-top: 0.5rem;
  padding-top: 0.5rem;
  border-top: 1px solid rgba(255, 255, 255, 0.07);
}
.styles-module__light___ORIft .styles-module__paletteSection___V8DEA + .styles-module__paletteSection___V8DEA {
  border-top-color: rgba(0, 0, 0, 0.07);
}

.styles-module__paletteSectionTitle___PqnjX {
  font-size: 0.6875rem;
  font-weight: 500;
  color: rgba(255, 255, 255, 0.5);
  letter-spacing: -0.0094em;
  padding: 0 0 3px 3px;
}
.styles-module__light___ORIft .styles-module__paletteSectionTitle___PqnjX {
  color: rgba(0, 0, 0, 0.4);
}

.styles-module__paletteItem___6TlnA {
  width: 100%;
  text-align: left;
  background: transparent;
  display: flex;
  align-items: center;
  gap: 0.375rem;
  padding: 0.25rem 0.25rem;
  margin-bottom: 1px;
  border-radius: 0.375rem;
  cursor: pointer;
  transition: background-color 0.15s ease, border-color 0.15s ease;
  border: 1px solid transparent;
  -webkit-user-select: none;
  user-select: none;
  min-height: 24px;
}
.styles-module__paletteItem___6TlnA:hover {
  background: rgba(255, 255, 255, 0.1);
}
.styles-module__paletteItem___6TlnA.styles-module__active___hosp7 {
  background: #3c82f7;
  border-color: transparent;
}
.styles-module__paletteItem___6TlnA.styles-module__wireframe___itvQU.styles-module__active___hosp7 {
  background: #f97316;
}
.styles-module__light___ORIft .styles-module__paletteItem___6TlnA:hover {
  background: rgba(0, 0, 0, 0.05);
}
.styles-module__light___ORIft .styles-module__paletteItem___6TlnA.styles-module__active___hosp7 {
  background: #3c82f7;
  border-color: transparent;
}
.styles-module__light___ORIft .styles-module__paletteItem___6TlnA.styles-module__wireframe___itvQU.styles-module__active___hosp7 {
  background: #f97316;
}

.styles-module__paletteItemIcon___0NPQK {
  width: 20px;
  height: 16px;
  border-radius: 2px;
  border: 1px dashed rgba(255, 255, 255, 0.15);
  background: rgba(255, 255, 255, 0.04);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  overflow: hidden;
  color: rgba(255, 255, 255, 0.45);
}
.styles-module__paletteItemIcon___0NPQK svg {
  display: block;
  width: 20px;
  height: 16px;
}
.styles-module__active___hosp7 .styles-module__paletteItemIcon___0NPQK {
  border-color: rgba(255, 255, 255, 0.3);
  background: rgba(255, 255, 255, 0.15);
  color: #fff;
}
.styles-module__light___ORIft .styles-module__paletteItemIcon___0NPQK {
  border-color: rgba(0, 0, 0, 0.12);
  background: rgba(0, 0, 0, 0.02);
  color: rgba(0, 0, 0, 0.4);
}
.styles-module__light___ORIft .styles-module__active___hosp7 .styles-module__paletteItemIcon___0NPQK {
  border-color: rgba(255, 255, 255, 0.3);
  background: rgba(255, 255, 255, 0.15);
  color: #fff;
}

.styles-module__paletteItemLabel___6ncO4 {
  font-size: 0.8125rem;
  font-weight: 500;
  color: rgba(255, 255, 255, 0.85);
  letter-spacing: -0.0094em;
  line-height: 1;
  min-width: 0;
}
.styles-module__active___hosp7 .styles-module__paletteItemLabel___6ncO4 {
  color: #fff;
  font-weight: 600;
}
.styles-module__light___ORIft .styles-module__paletteItemLabel___6ncO4 {
  color: rgba(0, 0, 0, 0.7);
}
.styles-module__light___ORIft .styles-module__active___hosp7 .styles-module__paletteItemLabel___6ncO4 {
  color: #fff;
  font-weight: 600;
}

.styles-module__placeScroll___7sClM {
  max-height: 240px;
  overflow-y: auto;
  overflow-x: hidden;
  padding-top: 0.25rem;
}
.styles-module__placeScroll___7sClM.styles-module__fadeTop___KT9tF {
  -webkit-mask-image: linear-gradient(to bottom, transparent 0, black 32px);
  mask-image: linear-gradient(to bottom, transparent 0, black 32px);
}
.styles-module__placeScroll___7sClM.styles-module__fadeBottom___x3ShT {
  -webkit-mask-image: linear-gradient(to bottom, black calc(100% - 32px), transparent 100%);
  mask-image: linear-gradient(to bottom, black calc(100% - 32px), transparent 100%);
}
.styles-module__placeScroll___7sClM.styles-module__fadeTop___KT9tF.styles-module__fadeBottom___x3ShT {
  -webkit-mask-image: linear-gradient(to bottom, transparent 0, black 32px, black calc(100% - 32px), transparent 100%);
  mask-image: linear-gradient(to bottom, transparent 0, black 32px, black calc(100% - 32px), transparent 100%);
}
.styles-module__placeScroll___7sClM::-webkit-scrollbar {
  width: 3px;
}
.styles-module__placeScroll___7sClM::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.12);
  border-radius: 2px;
}
.styles-module__light___ORIft .styles-module__placeScroll___7sClM::-webkit-scrollbar-thumb {
  background: rgba(0, 0, 0, 0.1);
}

.styles-module__paletteFooterWrap___71-fI {
  display: grid;
  grid-template-rows: 1fr;
  transition: grid-template-rows 0.25s cubic-bezier(0.32, 0.72, 0, 1);
}
.styles-module__paletteFooterWrap___71-fI.styles-module__footerHidden___fJUik {
  grid-template-rows: 0fr;
}

.styles-module__paletteFooterInnerContent___VC26h {
  opacity: 1;
  transform: translateY(0);
  transition: opacity 0.15s ease, transform 0.15s ease;
}
.styles-module__footerHidden___fJUik .styles-module__paletteFooterInnerContent___VC26h {
  opacity: 0;
  transform: translateY(4px);
}

.styles-module__paletteFooterInner___dfylY {
  overflow: hidden;
}

.styles-module__paletteFooter___QYnAG {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 24px;
  padding: 0 1rem;
  margin-top: 0.5rem;
  padding-top: 0.5rem;
  border-top: 1px solid rgba(255, 255, 255, 0.07);
}
.styles-module__light___ORIft .styles-module__paletteFooter___QYnAG {
  border-top-color: rgba(0, 0, 0, 0.07);
}

.styles-module__paletteFooterCount___D3Fia {
  font-size: 0.8125rem;
  font-weight: 400;
  letter-spacing: -0.0094em;
  color: rgba(255, 255, 255, 0.5);
}
.styles-module__light___ORIft .styles-module__paletteFooterCount___D3Fia {
  color: rgba(0, 0, 0, 0.5);
}

.styles-module__paletteFooterClear___ybBoa {
  font-size: 0.8125rem;
  font-weight: 400;
  letter-spacing: -0.0094em;
  color: rgba(255, 255, 255, 0.5);
  background: none;
  border: none;
  cursor: pointer;
  padding: 0;
  font-family: inherit;
  transition: color 0.15s ease;
}
.styles-module__paletteFooterClear___ybBoa:hover {
  color: rgba(255, 255, 255, 0.7);
}
.styles-module__light___ORIft .styles-module__paletteFooterClear___ybBoa {
  color: rgba(0, 0, 0, 0.5);
}
.styles-module__light___ORIft .styles-module__paletteFooterClear___ybBoa:hover {
  color: rgba(0, 0, 0, 0.6);
}

.styles-module__paletteFooterActions___fLzv8 {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.styles-module__rollingWrap___S75jM {
  display: inline-block;
  overflow: hidden;
  height: 1.15em;
  position: relative;
  vertical-align: bottom;
}

.styles-module__rollingNum___1RKDx {
  position: absolute;
  left: 0;
  top: 0;
}

.styles-module__exitUp___AFDRW {
  animation: styles-module__numExitUp___FRQqx 0.25s cubic-bezier(0.32, 0.72, 0, 1) forwards;
}

.styles-module__enterUp___CPlXb {
  animation: styles-module__numEnterUp___2Yd-w 0.25s cubic-bezier(0.32, 0.72, 0, 1) forwards;
}

.styles-module__exitDown___-1yAy {
  animation: styles-module__numExitDown___xm5by 0.25s cubic-bezier(0.32, 0.72, 0, 1) forwards;
}

.styles-module__enterDown___DDuFR {
  animation: styles-module__numEnterDown___hpxBk 0.25s cubic-bezier(0.32, 0.72, 0, 1) forwards;
}

@keyframes styles-module__numExitUp___FRQqx {
  from {
    transform: translateY(0);
    opacity: 1;
  }
  to {
    transform: translateY(-110%);
    opacity: 0;
  }
}
@keyframes styles-module__numEnterUp___2Yd-w {
  from {
    transform: translateY(110%);
    opacity: 0;
  }
  to {
    transform: translateY(0);
    opacity: 1;
  }
}
@keyframes styles-module__numExitDown___xm5by {
  from {
    transform: translateY(0);
    opacity: 1;
  }
  to {
    transform: translateY(110%);
    opacity: 0;
  }
}
@keyframes styles-module__numEnterDown___hpxBk {
  from {
    transform: translateY(-110%);
    opacity: 0;
  }
  to {
    transform: translateY(0);
    opacity: 1;
  }
}
.styles-module__rearrangeOverlay___-3R3t {
  position: fixed;
  inset: 0;
  z-index: 99995;
  pointer-events: none;
  cursor: default;
  -webkit-user-select: none;
  user-select: none;
  animation: styles-module__overlayFadeIn___aECVy 0.15s ease;
}

.styles-module__hoverHighlight___8eT-v {
  position: fixed;
  pointer-events: none;
  z-index: 99994;
  border: 2px dashed rgba(59, 130, 246, 0.5);
  border-radius: 4px;
  background: rgba(59, 130, 246, 0.06);
  animation: styles-module__highlightFadeIn___Lg7KY 0.12s ease;
}

.styles-module__sectionOutline___s0hy- {
  position: fixed;
  border: 2px solid;
  border-radius: 4px;
  cursor: grab;
}
.styles-module__sectionOutline___s0hy-:active {
  cursor: grabbing;
}
.styles-module__sectionOutline___s0hy- {
  transition: box-shadow 0.15s, border-color 0.3s, background-color 0.3s, border-style 0s;
  -webkit-user-select: none;
  user-select: none;
  pointer-events: auto;
  animation: styles-module__sectionEnter___-8BXT 0.2s ease;
}
.styles-module__sectionOutline___s0hy-:hover {
  box-shadow: 0 0 0 1px rgba(255, 255, 255, 0.1), 0 4px 12px rgba(0, 0, 0, 0.15);
}
.styles-module__sectionOutline___s0hy-.styles-module__selected___6yrp6 {
  border-style: solid;
  box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.15), 0 2px 8px rgba(59, 130, 246, 0.15);
}
.styles-module__sectionOutline___s0hy-.styles-module__selected___6yrp6:hover {
  box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.15), 0 2px 8px rgba(59, 130, 246, 0.15);
}
.styles-module__sectionOutline___s0hy-.styles-module__settled___b5U5o:not(.styles-module__selected___6yrp6) {
  border: 1.5px dashed rgba(150, 150, 150, 0.35);
  background-color: transparent !important;
  box-shadow: none;
}
.styles-module__sectionOutline___s0hy-.styles-module__settled___b5U5o:not(.styles-module__selected___6yrp6):hover {
  border-color: rgba(150, 150, 150, 0.6);
  box-shadow: none;
}
.styles-module__sectionOutline___s0hy-.styles-module__settled___b5U5o:not(.styles-module__selected___6yrp6) .styles-module__sectionLabel___F80HQ {
  opacity: 0;
  transition: opacity 0.15s ease;
}
.styles-module__sectionOutline___s0hy-.styles-module__settled___b5U5o:not(.styles-module__selected___6yrp6):hover .styles-module__sectionLabel___F80HQ {
  opacity: 1;
}
.styles-module__sectionOutline___s0hy-.styles-module__settled___b5U5o:not(.styles-module__selected___6yrp6) .styles-module__movedBadge___s8z-q,
.styles-module__sectionOutline___s0hy-.styles-module__settled___b5U5o:not(.styles-module__selected___6yrp6) .styles-module__sectionDimensions___RcJSL {
  opacity: 0;
  transition: opacity 0.15s ease;
}
.styles-module__sectionOutline___s0hy-.styles-module__settled___b5U5o:not(.styles-module__selected___6yrp6):hover .styles-module__sectionDimensions___RcJSL {
  opacity: 1;
}
.styles-module__sectionOutline___s0hy-.styles-module__exiting___YrM8F {
  opacity: 0;
  transform: scale(0.97);
  pointer-events: none;
  animation: none;
  transition: opacity 0.2s ease, transform 0.2s cubic-bezier(0.32, 0.72, 0, 1);
}

.styles-module__sectionLabel___F80HQ {
  position: absolute;
  top: 4px;
  left: 4px;
  font-size: 10px;
  font-weight: 600;
  color: #fff;
  padding: 2px 8px;
  border-radius: 4px;
  white-space: nowrap;
  pointer-events: none;
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.2);
  max-width: calc(100% - 8px);
  overflow: hidden;
  text-overflow: ellipsis;
}

.styles-module__movedBadge___s8z-q {
  position: absolute;
  bottom: 22px;
  right: 4px;
  font-size: 9px;
  font-weight: 700;
  color: #fff;
  background: #22c55e;
  padding: 2px 6px;
  border-radius: 4px;
  white-space: nowrap;
  pointer-events: none;
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.2);
  opacity: 0;
  transform: scale(0.8);
  transition: opacity 0.15s ease, transform 0.15s ease;
}
.styles-module__movedBadge___s8z-q.styles-module__badgeVisible___npbdS {
  opacity: 1;
  transform: scale(1);
  transition: opacity 0.2s cubic-bezier(0.34, 1.2, 0.64, 1), transform 0.2s cubic-bezier(0.34, 1.2, 0.64, 1);
}

.styles-module__resizedBadge___u51V8 {
  background: #3c82f7;
  bottom: 40px;
}

.styles-module__sectionDimensions___RcJSL {
  position: absolute;
  bottom: 4px;
  right: 4px;
  font-size: 9px;
  font-weight: 500;
  color: rgba(255, 255, 255, 0.7);
  background: rgba(0, 0, 0, 0.5);
  padding: 1px 5px;
  border-radius: 3px;
  white-space: nowrap;
  pointer-events: none;
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
}
.styles-module__light___ORIft .styles-module__sectionDimensions___RcJSL {
  color: rgba(0, 0, 0, 0.5);
  background: rgba(255, 255, 255, 0.7);
}

.styles-module__wireframeNotice___4GJyB {
  position: fixed;
  bottom: 16px;
  left: 24px;
  z-index: 99995;
  font-size: 9.5px;
  font-weight: 400;
  color: rgba(0, 0, 0, 0.4);
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
  pointer-events: auto;
  animation: styles-module__overlayFadeIn___aECVy 0.3s ease;
  line-height: 1.5;
  max-width: 280px;
}

.styles-module__wireframeOpacityRow___CJXzi {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}

.styles-module__wireframeOpacityLabel___afkfT {
  font-size: 9px;
  font-weight: 500;
  color: rgba(0, 0, 0, 0.32);
  letter-spacing: 0.02em;
  white-space: nowrap;
  -webkit-user-select: none;
  user-select: none;
}

.styles-module__wireframeOpacitySlider___YcoEs {
  -webkit-appearance: none;
  appearance: none;
  width: 56px;
  height: 4px;
  background: rgba(0, 0, 0, 0.08);
  border-radius: 2px;
  outline: none;
  cursor: pointer;
  flex-shrink: 0;
  transition: background 0.15s ease;
}
.styles-module__wireframeOpacitySlider___YcoEs:hover {
  background: rgba(0, 0, 0, 0.13);
}
.styles-module__wireframeOpacitySlider___YcoEs::-webkit-slider-thumb {
  -webkit-appearance: none;
  appearance: none;
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: #f97316;
  cursor: pointer;
  transition: background 0.15s ease;
}
.styles-module__wireframeOpacitySlider___YcoEs::-webkit-slider-thumb:hover {
  background: rgb(88.0082041185%, 37.39404381%, 2.2663056855%);
}
.styles-module__wireframeOpacitySlider___YcoEs::-moz-range-thumb {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: #f97316;
  border: none;
  cursor: pointer;
}
.styles-module__wireframeOpacitySlider___YcoEs::-moz-range-track {
  background: rgba(0, 0, 0, 0.08);
  height: 4px;
  border-radius: 2px;
}

.styles-module__wireframeNoticeTitleRow___PJqyG {
  display: flex;
  align-items: center;
  gap: 0;
  margin-bottom: 2px;
}

.styles-module__wireframeNoticeTitle___okr08 {
  font-weight: 600;
  color: rgba(0, 0, 0, 0.55);
}

.styles-module__wireframeNoticeDivider___PNKQ6 {
  width: 1px;
  height: 8px;
  background: rgba(0, 0, 0, 0.12);
  margin: 0 8px;
  flex-shrink: 0;
}

.styles-module__wireframeStartOver___YFk-I {
  font-size: 9.5px;
  font-weight: 500;
  color: rgba(0, 0, 0, 0.35);
  cursor: pointer;
  background: none;
  border: none;
  padding: 0;
  font-family: inherit;
  text-decoration: none;
  transition: color 0.12s ease;
  white-space: nowrap;
}
.styles-module__wireframeStartOver___YFk-I:hover {
  color: rgba(0, 0, 0, 0.6);
}

.styles-module__ghostOutline___po-kO {
  position: fixed;
  border: 1.5px dashed rgba(59, 130, 246, 0.4);
  border-radius: 4px;
  background: rgba(59, 130, 246, 0.04);
  cursor: grab;
  opacity: 0.5;
  -webkit-user-select: none;
  user-select: none;
  pointer-events: auto;
  animation: styles-module__ghostEnter___EC3Mb 0.25s ease;
  transition: box-shadow 0.15s, border-color 0.3s, opacity 0.25s;
}
.styles-module__ghostOutline___po-kO:active {
  cursor: grabbing;
}
.styles-module__ghostOutline___po-kO:hover {
  opacity: 0.7;
  box-shadow: 0 0 0 1px rgba(59, 130, 246, 0.1), 0 4px 12px rgba(0, 0, 0, 0.08);
}
.styles-module__ghostOutline___po-kO.styles-module__selected___6yrp6 {
  opacity: 1;
  border-style: solid;
  border-width: 2px;
  border-color: #3c82f7;
  background: rgba(59, 130, 246, 0.08);
  box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.15), 0 2px 8px rgba(59, 130, 246, 0.15);
}
.styles-module__ghostOutline___po-kO.styles-module__exiting___YrM8F {
  opacity: 0;
  transform: scale(0.97);
  pointer-events: none;
  animation: none;
  transition: opacity 0.2s ease, transform 0.2s cubic-bezier(0.32, 0.72, 0, 1);
}

.styles-module__ghostBadge___tsQUK {
  position: absolute;
  bottom: calc(100% + 4px);
  left: -1px;
  font-size: 9px;
  font-weight: 600;
  color: rgba(59, 130, 246, 0.9);
  background: rgba(59, 130, 246, 0.08);
  border: 1px solid rgba(59, 130, 246, 0.2);
  padding: 1px 5px;
  border-radius: 3px;
  white-space: nowrap;
  pointer-events: none;
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
  letter-spacing: 0.02em;
  line-height: 1.2;
  animation: styles-module__badgeSlideIn___typJ7 0.2s ease both;
}

@keyframes styles-module__badgeSlideIn___typJ7 {
  from {
    opacity: 0;
    transform: translateY(4px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}
.styles-module__ghostBadgeExtra___6CVoD {
  display: inline;
  animation: styles-module__badgeExtraIn___i4W8F 0.2s ease both;
}

@keyframes styles-module__badgeExtraIn___i4W8F {
  from {
    opacity: 0;
  }
  to {
    opacity: 1;
  }
}
.styles-module__originalOutline___Y6DD1 {
  position: fixed;
  border: 1.5px dashed rgba(150, 150, 150, 0.3);
  border-radius: 4px;
  background: transparent;
  pointer-events: none;
  -webkit-user-select: none;
  user-select: none;
  animation: styles-module__sectionEnter___-8BXT 0.2s ease;
}

.styles-module__originalLabel___HqI9g {
  position: absolute;
  top: 4px;
  left: 4px;
  font-size: 9px;
  font-weight: 500;
  color: rgba(150, 150, 150, 0.5);
  padding: 1px 6px;
  border-radius: 3px;
  white-space: nowrap;
  pointer-events: none;
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
  background: rgba(150, 150, 150, 0.08);
}

.styles-module__connectorSvg___Lovld {
  position: fixed;
  inset: 0;
  width: 100vw;
  height: 100vh;
  pointer-events: none;
  z-index: 99996;
}

.styles-module__connectorLine___XeWh- {
  transition: opacity 0.2s ease;
  animation: styles-module__connectorDraw___8sK5I 0.3s ease both;
}

.styles-module__connectorDot___yvf7C {
  transform-box: fill-box;
  transform-origin: center;
  animation: styles-module__connectorDotIn___NwTUq 0.25s cubic-bezier(0.34, 1.56, 0.64, 1) 0.15s both;
}

@keyframes styles-module__connectorDraw___8sK5I {
  from {
    opacity: 0;
  }
  to {
    opacity: 1;
  }
}
@keyframes styles-module__connectorDotIn___NwTUq {
  from {
    transform: scale(0);
    opacity: 0;
  }
  to {
    transform: scale(1);
    opacity: 1;
  }
}
.styles-module__connectorExiting___2lLOs {
  animation: styles-module__connectorOut___5QoPl 0.2s ease forwards;
}
.styles-module__connectorExiting___2lLOs .styles-module__connectorDot___yvf7C {
  animation: styles-module__connectorDotOut___FEq7e 0.2s ease forwards;
}

@keyframes styles-module__connectorOut___5QoPl {
  from {
    opacity: 1;
  }
  to {
    opacity: 0;
  }
}
@keyframes styles-module__connectorDotOut___FEq7e {
  from {
    transform: scale(1);
    opacity: 1;
  }
  to {
    transform: scale(0);
    opacity: 0;
  }
}
@keyframes styles-module__placementEnter___TdRhf {
  from {
    opacity: 0;
    transform: scale(0.85);
  }
  to {
    opacity: 1;
    transform: scale(1);
  }
}
@keyframes styles-module__sectionEnter___-8BXT {
  from {
    opacity: 0;
    transform: scale(0.96);
  }
  to {
    opacity: 1;
    transform: scale(1);
  }
}
@keyframes styles-module__highlightFadeIn___Lg7KY {
  from {
    opacity: 0;
  }
  to {
    opacity: 1;
  }
}
@keyframes styles-module__overlayFadeIn___aECVy {
  from {
    opacity: 0;
  }
  to {
    opacity: 1;
  }
}
@keyframes styles-module__ghostEnter___EC3Mb {
  from {
    opacity: 0;
    transform: scale(0.96);
  }
  to {
    opacity: 0.6;
    transform: scale(1);
  }
}
.styles-module__canvasToggle___-QqSy:focus-visible,
.styles-module__paletteItem___6TlnA:focus-visible {
  outline: 2px solid var(--agentation-color-accent);
  outline-offset: -2px;
}`,D={overlay:"styles-module__overlay___aWh-q",rearrangeOverlay:"styles-module__rearrangeOverlay___-3R3t",overlayExiting:"styles-module__overlayExiting___iEmYr",overlayFadeIn:"styles-module__overlayFadeIn___aECVy",light:"styles-module__light___ORIft",wireframe:"styles-module__wireframe___itvQU",placing:"styles-module__placing___45yD8",passthrough:"styles-module__passthrough___xaFeE",blankCanvas:"styles-module__blankCanvas___t2Eue",visible:"styles-module__visible___OKKqX",gridActive:"styles-module__gridActive___OZ-cf",paletteHeader:"styles-module__paletteHeader___-Q5gQ",paletteHeaderTitle:"styles-module__paletteHeaderTitle___oHqZC",paletteHeaderDesc:"styles-module__paletteHeaderDesc___6i74T",wireframePurposeWrap:"styles-module__wireframePurposeWrap___To-tS",collapsed:"styles-module__collapsed___Ms9vS",wireframePurposeInner:"styles-module__wireframePurposeInner___Lrahs",wireframePurposeInput:"styles-module__wireframePurposeInput___7EtBN",canvasToggle:"styles-module__canvasToggle___-QqSy",active:"styles-module__active___hosp7",canvasToggleIcon:"styles-module__canvasToggleIcon___7pJ82",canvasToggleLabel:"styles-module__canvasToggleLabel___OanpY",placement:"styles-module__placement___zcxv8",placementEnter:"styles-module__placementEnter___TdRhf",selected:"styles-module__selected___6yrp6",dragging:"styles-module__dragging___le6KZ",exiting:"styles-module__exiting___YrM8F",placementContent:"styles-module__placementContent___f64A4",placementLabel:"styles-module__placementLabel___0KvWl",placementAnnotation:"styles-module__placementAnnotation___78pTr",annotationVisible:"styles-module__annotationVisible___mrUyA",sectionAnnotation:"styles-module__sectionAnnotation___aUIs0",handle:"styles-module__handle___Ikbxm",sectionOutline:"styles-module__sectionOutline___s0hy-",ghostOutline:"styles-module__ghostOutline___po-kO",handleNw:"styles-module__handleNw___4TMIj",handleNe:"styles-module__handleNe___mnsTh",handleSe:"styles-module__handleSe___oSFnk",handleSw:"styles-module__handleSw___pi--Z",handleN:"styles-module__handleN___aBA-Q",handleE:"styles-module__handleE___0hM5u",handleS:"styles-module__handleS___JjDRv",handleW:"styles-module__handleW___ERWGQ",edgeHandle:"styles-module__edgeHandle___XxXdT",edgeN:"styles-module__edgeN___-JJDj",edgeS:"styles-module__edgeS___66lMX",edgeE:"styles-module__edgeE___1bGDa",edgeW:"styles-module__edgeW___lHQNo",deleteButton:"styles-module__deleteButton___LkGCb",drawBox:"styles-module__drawBox___BrVAa",selectBox:"styles-module__selectBox___Iu8kB",sizeIndicator:"styles-module__sizeIndicator___7zJ4y",guideLine:"styles-module__guideLine___DUQY2",dragPreview:"styles-module__dragPreview___onPbU",dragPreviewWireframe:"styles-module__dragPreviewWireframe___jsg0G",palette:"styles-module__palette___C7iSH",paletteItem:"styles-module__paletteItem___6TlnA",paletteItemLabel:"styles-module__paletteItemLabel___6ncO4",paletteSectionTitle:"styles-module__paletteSectionTitle___PqnjX",paletteFooter:"styles-module__paletteFooter___QYnAG",paletteSection:"styles-module__paletteSection___V8DEA",paletteItemIcon:"styles-module__paletteItemIcon___0NPQK",placeScroll:"styles-module__placeScroll___7sClM",fadeTop:"styles-module__fadeTop___KT9tF",fadeBottom:"styles-module__fadeBottom___x3ShT",paletteFooterWrap:"styles-module__paletteFooterWrap___71-fI",footerHidden:"styles-module__footerHidden___fJUik",paletteFooterInnerContent:"styles-module__paletteFooterInnerContent___VC26h",paletteFooterInner:"styles-module__paletteFooterInner___dfylY",paletteFooterCount:"styles-module__paletteFooterCount___D3Fia",paletteFooterClear:"styles-module__paletteFooterClear___ybBoa",paletteFooterActions:"styles-module__paletteFooterActions___fLzv8",rollingWrap:"styles-module__rollingWrap___S75jM",rollingNum:"styles-module__rollingNum___1RKDx",exitUp:"styles-module__exitUp___AFDRW",numExitUp:"styles-module__numExitUp___FRQqx",enterUp:"styles-module__enterUp___CPlXb",numEnterUp:"styles-module__numEnterUp___2Yd-w",exitDown:"styles-module__exitDown___-1yAy",numExitDown:"styles-module__numExitDown___xm5by",enterDown:"styles-module__enterDown___DDuFR",numEnterDown:"styles-module__numEnterDown___hpxBk",hoverHighlight:"styles-module__hoverHighlight___8eT-v",highlightFadeIn:"styles-module__highlightFadeIn___Lg7KY",sectionEnter:"styles-module__sectionEnter___-8BXT",settled:"styles-module__settled___b5U5o",sectionLabel:"styles-module__sectionLabel___F80HQ",movedBadge:"styles-module__movedBadge___s8z-q",sectionDimensions:"styles-module__sectionDimensions___RcJSL",badgeVisible:"styles-module__badgeVisible___npbdS",resizedBadge:"styles-module__resizedBadge___u51V8",wireframeNotice:"styles-module__wireframeNotice___4GJyB",wireframeOpacityRow:"styles-module__wireframeOpacityRow___CJXzi",wireframeOpacityLabel:"styles-module__wireframeOpacityLabel___afkfT",wireframeOpacitySlider:"styles-module__wireframeOpacitySlider___YcoEs",wireframeNoticeTitleRow:"styles-module__wireframeNoticeTitleRow___PJqyG",wireframeNoticeTitle:"styles-module__wireframeNoticeTitle___okr08",wireframeNoticeDivider:"styles-module__wireframeNoticeDivider___PNKQ6",wireframeStartOver:"styles-module__wireframeStartOver___YFk-I",ghostEnter:"styles-module__ghostEnter___EC3Mb",ghostBadge:"styles-module__ghostBadge___tsQUK",badgeSlideIn:"styles-module__badgeSlideIn___typJ7",ghostBadgeExtra:"styles-module__ghostBadgeExtra___6CVoD",badgeExtraIn:"styles-module__badgeExtraIn___i4W8F",originalOutline:"styles-module__originalOutline___Y6DD1",originalLabel:"styles-module__originalLabel___HqI9g",connectorSvg:"styles-module__connectorSvg___Lovld",connectorLine:"styles-module__connectorLine___XeWh-",connectorDraw:"styles-module__connectorDraw___8sK5I",connectorDot:"styles-module__connectorDot___yvf7C",connectorDotIn:"styles-module__connectorDotIn___NwTUq",connectorExiting:"styles-module__connectorExiting___2lLOs",connectorOut:"styles-module__connectorOut___5QoPl",connectorDotOut:"styles-module__connectorDotOut___FEq7e"},Vi=24,pc=5;function O0(e,t,n,o,r){let i=1/0,l=1/0,s=e.x,a=e.x+e.width,c=e.x+e.width/2,f=e.y,u=e.y+e.height,x=e.y+e.height/2,S=!o,b=S?[s,a,c]:[...o.left?[s]:[],...o.right?[a]:[]],N=S?[f,u,x]:[...o.top?[f]:[],...o.bottom?[u]:[]],E=[];for(let se of t)n.has(se.id)||E.push(se);r&&E.push(...r);for(let se of E){let Z=se.x,fe=se.x+se.width,oe=se.x+se.width/2,ae=se.y,pe=se.y+se.height,kt=se.y+se.height/2;for(let Oe of b)for(let qe of[Z,fe,oe]){let Ae=qe-Oe;Math.abs(Ae)<pc&&Math.abs(Ae)<Math.abs(i)&&(i=Ae)}for(let Oe of N)for(let qe of[ae,pe,kt]){let Ae=qe-Oe;Math.abs(Ae)<pc&&Math.abs(Ae)<Math.abs(l)&&(l=Ae)}}let g=Math.abs(i)<pc?i:0,v=Math.abs(l)<pc?l:0,h=[],C=new Set,U=s+g,V=a+g,B=c+g,q=f+v,F=u+v,K=x+v;for(let se of E){let Z=se.x,fe=se.x+se.width,oe=se.x+se.width/2,ae=se.y,pe=se.y+se.height,kt=se.y+se.height/2;for(let Oe of[Z,oe,fe])for(let qe of[U,B,V])if(Math.abs(qe-Oe)<.5){let Ae=`x:${Math.round(Oe)}`;C.has(Ae)||(C.add(Ae),h.push({axis:"x",pos:Oe}))}for(let Oe of[ae,kt,pe])for(let qe of[q,K,F])if(Math.abs(qe-Oe)<.5){let Ae=`y:${Math.round(Oe)}`;C.has(Ae)||(C.add(Ae),h.push({axis:"y",pos:Oe}))}}return{dx:g,dy:v,guides:h}}function A0(){return`dp-${Date.now()}-${Math.random().toString(36).slice(2,7)}`}function Kx({placements:e,onChange:t,activeComponent:n,onActiveComponentChange:o,isDarkMode:r,exiting:i,onInteractionChange:l,className:s,passthrough:a,extraSnapRects:c,onSelectionChange:f,deselectSignal:u,onDragMove:x,onDragEnd:S,clearingPlacements:b,wireframe:N}){let[E,g]=(0,ot.useState)(new Set),[v,h]=(0,ot.useState)(null),[C,U]=(0,ot.useState)(null),[V,B]=(0,ot.useState)(null),[q,F]=(0,ot.useState)([]),[K,se]=(0,ot.useState)(null),[Z,fe]=(0,ot.useState)(!1),oe=(0,ot.useRef)(!1),[ae,pe]=(0,ot.useState)(new Set),kt=(0,ot.useRef)(new Map),Oe=(0,ot.useRef)(null),qe=(0,ot.useRef)(null),Ae=(0,ot.useRef)(e);Ae.current=e;let _t=(0,ot.useRef)(f);_t.current=f;let qt=(0,ot.useRef)(x);qt.current=x;let wn=(0,ot.useRef)(S);wn.current=S;let Re=(0,ot.useRef)(u);(0,ot.useEffect)(()=>{u!==Re.current&&(Re.current=u,g(new Set))},[u]),(0,ot.useEffect)(()=>{b?.length&&(g(G=>new Set([...G].filter(he=>!b.some(Ne=>Ne.id===he)))),qe.current=null)},[b]),(0,ot.useEffect)(()=>{let G=he=>{let Ne=he.composedPath()[0]||he.target;if(!(Ne.tagName==="INPUT"||Ne.tagName==="TEXTAREA"||Ne.isContentEditable)){if((he.key==="Backspace"||he.key==="Delete")&&E.size>0){he.preventDefault();let be=new Set(E);pe(be),g(new Set),nt(()=>{t(Ae.current.filter(Ke=>!be.has(Ke.id))),pe(new Set)},180);return}if(["ArrowUp","ArrowDown","ArrowLeft","ArrowRight"].includes(he.key)&&E.size>0){he.preventDefault();let be=he.shiftKey?20:1,Ke=he.key==="ArrowLeft"?-be:he.key==="ArrowRight"?be:0,We=he.key==="ArrowUp"?-be:he.key==="ArrowDown"?be:0;t(e.map(Qe=>E.has(Qe.id)?{...Qe,x:Math.max(0,Qe.x+Ke),y:Math.max(0,Qe.y+We)}:Qe));return}if(he.key==="Escape"){n?o(null):E.size>0&&g(new Set);return}}};return document.addEventListener("keydown",G),()=>document.removeEventListener("keydown",G)},[E,n,e,t,o]);let Zn=(0,ot.useCallback)(G=>{if(G.button!==0||a||G.target.closest(`.${D.placement}`))return;G.preventDefault(),G.stopPropagation();let Ne=window.scrollY,Me=G.clientX,be=G.clientY;if(n){qe.current="place",l?.(!0);let Ke=!1,We=Me,Qe=be,Ye=L=>{We=L.clientX,Qe=L.clientY;let R=Math.abs(We-Me),z=Math.abs(Qe-be);if((R>5||z>5)&&(Ke=!0),Ke){let j=Math.min(Me,We),ee=Math.min(be,Qe),te=Math.abs(We-Me),Y=Math.abs(Qe-be);h({x:j,y:ee,w:te,h:Y}),B({x:L.clientX+12,y:L.clientY+12,text:`${Math.round(te)} \xD7 ${Math.round(Y)}`})}},H=L=>{window.removeEventListener("mousemove",Ye),window.removeEventListener("mouseup",H),h(null),B(null),qe.current=null,l?.(!1);let R=ne[n],z,j,ee,te;Ke?(z=Math.min(Me,We),j=Math.min(be,Qe)+Ne,ee=Math.max(Vi,Math.abs(We-Me)),te=Math.max(Vi,Math.abs(Qe-be))):(ee=R.width,te=R.height,z=Me-ee/2,j=be+Ne-te/2),z=Math.max(0,z),j=Math.max(0,j);let Y={id:A0(),type:n,x:z,y:j,width:ee,height:te,scrollY:Ne,timestamp:Date.now()},de=[...e,Y];t(de),g(new Set([Y.id])),o(null)};window.addEventListener("mousemove",Ye),window.addEventListener("mouseup",H)}else{G.shiftKey||g(new Set),qe.current="select";let Ke=!1,We=Ye=>{let H=Math.abs(Ye.clientX-Me),L=Math.abs(Ye.clientY-be);if((H>4||L>4)&&(Ke=!0),Ke){let R=Math.min(Me,Ye.clientX),z=Math.min(be,Ye.clientY);U({x:R,y:z,w:Math.abs(Ye.clientX-Me),h:Math.abs(Ye.clientY-be)})}},Qe=Ye=>{if(window.removeEventListener("mousemove",We),window.removeEventListener("mouseup",Qe),qe.current=null,Ke){let H=Math.min(Me,Ye.clientX),L=Math.min(be,Ye.clientY)+Ne,R=Math.abs(Ye.clientX-Me),z=Math.abs(Ye.clientY-be),j=new Set(G.shiftKey?E:new Set);for(let ee of e){let te=ee.y-Ne;ee.x+ee.width>H&&ee.x<H+R&&ee.y+ee.height>L&&ee.y<L+z&&j.add(ee.id)}g(j)}U(null)};window.addEventListener("mousemove",We),window.addEventListener("mouseup",Qe)}},[n,a,e,t,E]),Ro=(0,ot.useCallback)((G,he)=>{if(G.button!==0)return;let Ne=G.target;if(Ne.closest(`.${D.handle}`)||Ne.closest(`.${D.deleteButton}`))return;G.preventDefault(),G.stopPropagation();let Me;G.shiftKey?(Me=new Set(E),Me.has(he)?Me.delete(he):Me.add(he)):E.has(he)?Me=new Set(E):Me=new Set([he]),g(Me),(Me.size!==E.size||[...Me].some(de=>!E.has(de)))&&_t.current?.(Me,G.shiftKey);let Ke=window.scrollY,We=G.clientX,Qe=G.clientY,Ye=new Map;for(let de of e)Me.has(de.id)&&Ye.set(de.id,{x:de.x,y:de.y});qe.current="move",l?.(!0);let H=!1,L=!1,R=e,z=0,j=0,ee=new Map;for(let de of e)Ye.has(de.id)&&ee.set(de.id,{w:de.width,h:de.height});let te=de=>{let ke=de.clientX-We,Te=de.clientY-Qe;if((Math.abs(ke)>2||Math.abs(Te)>2)&&(H=!0),!H)return;if(de.altKey&&!L){L=!0;let ft=[];for(let ct of e)Ye.has(ct.id)&&ft.push({...ct,id:A0(),timestamp:Date.now()});R=[...e,...ft]}let rt=1/0,st=1/0,Je=-1/0,je=-1/0;for(let[ft,ct]of Ye){let ie=ee.get(ft);ie&&(rt=Math.min(rt,ct.x+ke),st=Math.min(st,ct.y+Te),Je=Math.max(Je,ct.x+ke+ie.w),je=Math.max(je,ct.y+Te+ie.h))}let Ie={x:rt,y:st,width:Je-rt,height:je-st},{dx:gt,dy:mt,guides:at}=O0(Ie,R,new Set(Ye.keys()),void 0,c);F(at);let Pe=ke+gt,Ze=Te+mt;z=Pe,j=Ze,t(R.map(ft=>{let ct=Ye.get(ft.id);return ct?{...ft,x:Math.max(0,ct.x+Pe),y:Math.max(0,ct.y+Ze)}:ft})),qt.current?.(Pe,Ze)},Y=()=>{window.removeEventListener("mousemove",te),window.removeEventListener("mouseup",Y),qe.current=null,l?.(!1),F([]),wn.current?.(z,j,H)};window.addEventListener("mousemove",te),window.addEventListener("mouseup",Y)},[E,e,t,l]),qo=(0,ot.useCallback)((G,he,Ne)=>{G.preventDefault(),G.stopPropagation();let Me=e.find(j=>j.id===he);if(!Me)return;g(new Set([he])),qe.current="resize",l?.(!0);let be=G.clientX,Ke=G.clientY,We=Me.width,Qe=Me.height,Ye=Me.x,H=Me.y,L={left:Ne.includes("w"),right:Ne.includes("e"),top:Ne.includes("n"),bottom:Ne.includes("s")},R=j=>{let ee=j.clientX-be,te=j.clientY-Ke,Y=We,de=Qe,ke=Ye,Te=H;Ne.includes("e")&&(Y=Math.max(Vi,We+ee)),Ne.includes("w")&&(Y=Math.max(Vi,We-ee),ke=Ye+We-Y),Ne.includes("s")&&(de=Math.max(Vi,Qe+te)),Ne.includes("n")&&(de=Math.max(Vi,Qe-te),Te=H+Qe-de);let rt={x:ke,y:Te,width:Y,height:de},{dx:st,dy:Je,guides:je}=O0(rt,Ae.current,new Set([he]),L,c);F(je),st!==0&&(L.right?Y+=st:L.left&&(ke+=st,Y-=st)),Je!==0&&(L.bottom?de+=Je:L.top&&(Te+=Je,de-=Je)),t(Ae.current.map(Ie=>Ie.id===he?{...Ie,x:ke,y:Te,width:Y,height:de}:Ie)),B({x:j.clientX+12,y:j.clientY+12,text:`${Math.round(Y)} \xD7 ${Math.round(de)}`})},z=()=>{window.removeEventListener("mousemove",R),window.removeEventListener("mouseup",z),B(null),qe.current=null,l?.(!1),F([])};window.addEventListener("mousemove",R),window.addEventListener("mouseup",z)},[e,t,l]),Ko=(0,ot.useCallback)(G=>{qe.current=null,pe(he=>{let Ne=new Set(he);return Ne.add(G),Ne}),g(he=>{let Ne=new Set(he);return Ne.delete(G),Ne}),nt(()=>{t(Ae.current.filter(he=>he.id!==G)),pe(he=>{let Ne=new Set(he);return Ne.delete(G),Ne})},180)},[t]),$o=new Set(["text","hero","button","badge","cta","toast","modal","card","navigation","tabs","input","search","breadcrumb","pricing","testimonial","alert","banner","tag","notification","stat","productCard"]),To={hero:"Headline text",button:"Button label",badge:"Badge label",cta:"Call to action text",toast:"Notification message",modal:"Dialog title",card:"Card title",navigation:"Brand / nav items",tabs:"Tab labels",input:"Placeholder text",search:"Search placeholder",pricing:"Plan name or price",testimonial:"Quote text",alert:"Alert message",banner:"Banner text",tag:"Tag label",notification:"Notification message",stat:"Metric value",productCard:"Product name"},Po=(0,ot.useCallback)(G=>{let he=e.find(Ne=>Ne.id===G);he&&(oe.current=!!he.text,se(G),fe(!1))},[e]),Bt=(0,ot.useCallback)(()=>{K&&(fe(!0),nt(()=>{se(null),fe(!1)},150))},[K]);(0,ot.useEffect)(()=>{i&&K&&Bt()},[i]);let eo=(0,ot.useCallback)(G=>{K&&(t(e.map(he=>he.id===K?{...he,text:G.trim()||void 0}:he)),Bt())},[K,e,t,Bt]),mo=typeof window<"u"?window.scrollY:0,Er=["nw","ne","se","sw"],On=N?"#f97316":"#3c82f7",Do=[{dir:"n",cls:D.edgeN,arrow:Ot("svg",{width:"8",height:"6",viewBox:"0 0 8 6",fill:"none",children:Ot("path",{d:"M4 0.5L1 4.5h6z",fill:On})})},{dir:"e",cls:D.edgeE,arrow:Ot("svg",{width:"6",height:"8",viewBox:"0 0 6 8",fill:"none",children:Ot("path",{d:"M5.5 4L1.5 1v6z",fill:On})})},{dir:"s",cls:D.edgeS,arrow:Ot("svg",{width:"8",height:"6",viewBox:"0 0 8 6",fill:"none",children:Ot("path",{d:"M4 5.5L1 1.5h6z",fill:On})})},{dir:"w",cls:D.edgeW,arrow:Ot("svg",{width:"6",height:"8",viewBox:"0 0 6 8",fill:"none",children:Ot("path",{d:"M0.5 4L4.5 1v6z",fill:On})})}];return z0(qx,{children:[Ot("div",{ref:Oe,className:`${D.overlay} ${r?"":D.light} ${n?D.placing:""} ${a?D.passthrough:""} ${i?D.overlayExiting:""} ${N?D.wireframe:""}${s?` ${s}`:""}`,"data-feedback-toolbar":!0,onMouseDown:Zn,children:e.map(G=>{let he=E.has(G.id),Ne=po[G.type]?.label||G.type,Me=G.y-mo;return z0("div",{"data-design-placement":G.id,className:`${D.placement} ${he?D.selected:""} ${ae.has(G.id)||b?.includes(G)?D.exiting:""}`,style:{left:G.x,top:Me,width:G.width,height:G.height,position:"fixed"},onMouseDown:be=>Ro(be,G.id),onDoubleClick:()=>Po(G.id),children:[Ot("span",{className:D.placementLabel,children:Ne}),Ot("span",{className:`${D.placementAnnotation} ${G.text?D.annotationVisible:""}`,children:(G.text&&kt.current.set(G.id,G.text),G.text||kt.current.get(G.id)||"")}),Ot("div",{className:D.placementContent,children:Ot(Xx,{type:G.type,width:G.width,height:G.height,text:G.text})}),Ot("div",{className:D.deleteButton,onMouseDown:be=>be.stopPropagation(),onClick:()=>Ko(G.id),children:"\u2715"}),Er.map(be=>Ot("div",{className:`${D.handle} ${D[`handle${be.charAt(0).toUpperCase()}${be.slice(1)}`]}`,onMouseDown:Ke=>qo(Ke,G.id,be)},be)),Do.map(({dir:be,cls:Ke,arrow:We})=>Ot("div",{className:`${D.edgeHandle} ${Ke}`,onMouseDown:Qe=>qo(Qe,G.id,be),children:We},be))]},G.id)})}),K&&(()=>{let G=e.find(H=>H.id===K);if(!G)return null;let he=G.y-mo,Ne=G.x+G.width/2,Me=he-8,be=he+G.height+8,Ke=Me>200,We=be<window.innerHeight-100,Qe=Math.max(160,Math.min(window.innerWidth-160,Ne)),Ye;return Ke?Ye={left:Qe,bottom:window.innerHeight-Me}:We?Ye={left:Qe,top:be}:Ye={left:Qe,top:Math.max(80,window.innerHeight/2-80)},Ot(J_,{element:po[G.type]?.label||G.type,placeholder:To[G.type]||"Label or content text",initialValue:G.text??"",submitLabel:oe.current?"Save":"Set",onSubmit:eo,onCancel:Bt,onDelete:oe.current?()=>{eo("")}:void 0,isExiting:Z,lightMode:!r,style:Ye})})(),v&&Ot("div",{className:D.drawBox,style:{left:v.x,top:v.y,width:v.w,height:v.h},"data-feedback-toolbar":!0}),C&&Ot("div",{className:D.selectBox,style:{left:C.x,top:C.y,width:C.w,height:C.h},"data-feedback-toolbar":!0}),V&&Ot("div",{className:D.sizeIndicator,style:{left:V.x,top:V.y},"data-feedback-toolbar":!0,children:V.text}),q.map((G,he)=>Ot("div",{className:D.guideLine,style:G.axis==="x"?{position:"fixed",left:G.pos,top:0,width:1,bottom:0}:{position:"fixed",left:0,top:G.pos-mo,right:0,height:1},"data-feedback-toolbar":!0},`${G.axis}-${G.pos}-${he}`))]})}function Rg(e,{keepMounted:t=!1,onExited:n}={}){let[o,r]=(0,Mr.useState)(t||e),i=(0,Mr.useRef)(null),l=(0,Mr.useRef)(n);return e&&!o&&r(!0),(0,Mr.useLayoutEffect)(()=>{l.current=n},[n]),(0,Mr.useLayoutEffect)(()=>{let s=i.current;if(!s||s.dataset.panelOpen==="true"===e)return;getComputedStyle(s).opacity,s.dataset.panelPresent="true",s.dataset.panelOpen=String(e);let a=!1,c=s.getAnimations?.()??[];return Promise.allSettled(c.map(f=>f.finished)).then(()=>{a||e||(delete s.dataset.panelPresent,t||r(!1),l.current?.())}),()=>{a=!0}},[e,o,t]),{ref:i,mounted:o}}function Jx(e){if(!e)return"";let t=e.scrollTop>2,n=e.scrollTop+e.clientHeight<e.scrollHeight-2;return`${t?D.fadeTop:""} ${n?D.fadeBottom:""}`}var m="currentColor",P="0.5";function Zx({type:e}){switch(e){case"navigation":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"1",y:"4",width:"18",height:"8",rx:"1",stroke:m,strokeWidth:P}),_("rect",{x:"2.5",y:"7",width:"3",height:"1.5",rx:".5",fill:m,opacity:".4"}),_("rect",{x:"7",y:"7",width:"2.5",height:"1.5",rx:".5",fill:m,opacity:".25"}),_("rect",{x:"11",y:"7",width:"2.5",height:"1.5",rx:".5",fill:m,opacity:".25"})]});case"header":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"1",y:"2",width:"18",height:"12",rx:"1",stroke:m,strokeWidth:P}),_("rect",{x:"3",y:"5.5",width:"8",height:"2",rx:".5",fill:m,opacity:".35"}),_("rect",{x:"3",y:"9",width:"12",height:"1",rx:".5",fill:m,opacity:".15"})]});case"hero":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"1",y:"1",width:"18",height:"14",rx:"1",stroke:m,strokeWidth:P}),_("rect",{x:"5",y:"5",width:"10",height:"1.5",rx:".5",fill:m,opacity:".35"}),_("rect",{x:"7",y:"8",width:"6",height:"1",rx:".5",fill:m,opacity:".15"}),_("rect",{x:"7.5",y:"10.5",width:"5",height:"2.5",rx:"1",stroke:m,strokeWidth:P})]});case"section":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"1",y:"1",width:"18",height:"14",rx:"1",stroke:m,strokeWidth:P}),_("rect",{x:"3",y:"4",width:"6",height:"1",rx:".5",fill:m,opacity:".3"}),_("rect",{x:"3",y:"6.5",width:"14",height:"1",rx:".5",fill:m,opacity:".15"}),_("rect",{x:"3",y:"9",width:"10",height:"1",rx:".5",fill:m,opacity:".15"})]});case"sidebar":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"1",y:"1",width:"7",height:"14",rx:"1",stroke:m,strokeWidth:P}),_("rect",{x:"2.5",y:"4",width:"4",height:"1",rx:".5",fill:m,opacity:".3"}),_("rect",{x:"2.5",y:"6.5",width:"3.5",height:"1",rx:".5",fill:m,opacity:".15"}),_("rect",{x:"2.5",y:"9",width:"4",height:"1",rx:".5",fill:m,opacity:".15"})]});case"footer":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"1",y:"7",width:"18",height:"8",rx:"1",stroke:m,strokeWidth:P}),_("rect",{x:"3",y:"9.5",width:"4",height:"1",rx:".5",fill:m,opacity:".25"}),_("rect",{x:"9",y:"9.5",width:"4",height:"1",rx:".5",fill:m,opacity:".25"}),_("rect",{x:"15",y:"9.5",width:"3",height:"1",rx:".5",fill:m,opacity:".2"})]});case"modal":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"3",y:"2",width:"14",height:"12",rx:"1.5",stroke:m,strokeWidth:P}),_("rect",{x:"5",y:"4.5",width:"7",height:"1",rx:".5",fill:m,opacity:".3"}),_("rect",{x:"5",y:"7",width:"10",height:"1",rx:".5",fill:m,opacity:".15"}),_("rect",{x:"11",y:"11",width:"5",height:"2",rx:".75",stroke:m,strokeWidth:P})]});case"divider":return _("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:_("line",{x1:"2",y1:"8",x2:"18",y2:"8",stroke:m,strokeWidth:"0.5",opacity:".3"})});case"card":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"2",y:"1",width:"16",height:"14",rx:"1.5",stroke:m,strokeWidth:P}),_("rect",{x:"2",y:"1",width:"16",height:"5.5",rx:"1",fill:m,opacity:".04"}),_("rect",{x:"4",y:"8.5",width:"8",height:"1",rx:".5",fill:m,opacity:".25"}),_("rect",{x:"4",y:"11",width:"11",height:"1",rx:".5",fill:m,opacity:".12"})]});case"text":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"2",y:"4",width:"14",height:"1.5",rx:".5",fill:m,opacity:".3"}),_("rect",{x:"2",y:"7",width:"11",height:"1",rx:".5",fill:m,opacity:".15"}),_("rect",{x:"2",y:"9.5",width:"13",height:"1",rx:".5",fill:m,opacity:".15"}),_("rect",{x:"2",y:"12",width:"8",height:"1",rx:".5",fill:m,opacity:".12"})]});case"image":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"2",y:"2",width:"16",height:"12",rx:"1",stroke:m,strokeWidth:P}),_("line",{x1:"2",y1:"2",x2:"18",y2:"14",stroke:m,strokeWidth:".3",opacity:".25"}),_("line",{x1:"18",y1:"2",x2:"2",y2:"14",stroke:m,strokeWidth:".3",opacity:".25"})]});case"video":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"2",y:"2",width:"16",height:"12",rx:"1",stroke:m,strokeWidth:P}),_("path",{d:"M8.5 5.5v5l4.5-2.5z",stroke:m,strokeWidth:P,fill:m,opacity:".15"})]});case"table":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"1",y:"2",width:"18",height:"12",rx:"1",stroke:m,strokeWidth:P}),_("line",{x1:"1",y1:"5.5",x2:"19",y2:"5.5",stroke:m,strokeWidth:".3",opacity:".25"}),_("line",{x1:"1",y1:"9",x2:"19",y2:"9",stroke:m,strokeWidth:".3",opacity:".25"}),_("line",{x1:"7",y1:"2",x2:"7",y2:"14",stroke:m,strokeWidth:".3",opacity:".25"}),_("line",{x1:"13",y1:"2",x2:"13",y2:"14",stroke:m,strokeWidth:".3",opacity:".25"})]});case"grid":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"1.5",y:"2",width:"7",height:"5.5",rx:"1",stroke:m,strokeWidth:P}),_("rect",{x:"11.5",y:"2",width:"7",height:"5.5",rx:"1",stroke:m,strokeWidth:P}),_("rect",{x:"1.5",y:"9.5",width:"7",height:"5.5",rx:"1",stroke:m,strokeWidth:P}),_("rect",{x:"11.5",y:"9.5",width:"7",height:"5.5",rx:"1",stroke:m,strokeWidth:P})]});case"list":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("circle",{cx:"3.5",cy:"4.5",r:"1",stroke:m,strokeWidth:P}),_("rect",{x:"6.5",y:"4",width:"10",height:"1",rx:".5",fill:m,opacity:".2"}),_("circle",{cx:"3.5",cy:"8",r:"1",stroke:m,strokeWidth:P}),_("rect",{x:"6.5",y:"7.5",width:"8",height:"1",rx:".5",fill:m,opacity:".2"}),_("circle",{cx:"3.5",cy:"11.5",r:"1",stroke:m,strokeWidth:P}),_("rect",{x:"6.5",y:"11",width:"11",height:"1",rx:".5",fill:m,opacity:".2"})]});case"chart":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"3",y:"9",width:"2.5",height:"4",rx:".5",fill:m,opacity:".2"}),_("rect",{x:"7",y:"6",width:"2.5",height:"7",rx:".5",fill:m,opacity:".25"}),_("rect",{x:"11",y:"3",width:"2.5",height:"10",rx:".5",fill:m,opacity:".3"}),_("rect",{x:"15",y:"5",width:"2.5",height:"8",rx:".5",fill:m,opacity:".2"})]});case"accordion":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"1.5",y:"2",width:"17",height:"4",rx:"1",stroke:m,strokeWidth:P}),_("rect",{x:"3",y:"3.5",width:"6",height:"1",rx:".5",fill:m,opacity:".25"}),_("rect",{x:"1.5",y:"7.5",width:"17",height:"3",rx:"1",stroke:m,strokeWidth:P}),_("rect",{x:"1.5",y:"12",width:"17",height:"3",rx:"1",stroke:m,strokeWidth:P})]});case"carousel":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"3",y:"2",width:"14",height:"10",rx:"1",stroke:m,strokeWidth:P}),_("path",{d:"M1.5 7L3 8.5 1.5 10",stroke:m,strokeWidth:P,opacity:".35"}),_("path",{d:"M18.5 7L17 8.5 18.5 10",stroke:m,strokeWidth:P,opacity:".35"}),_("circle",{cx:"8.5",cy:"14",r:".6",fill:m,opacity:".35"}),_("circle",{cx:"10",cy:"14",r:".6",fill:m,opacity:".15"}),_("circle",{cx:"11.5",cy:"14",r:".6",fill:m,opacity:".15"})]});case"button":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"3",y:"5",width:"14",height:"6",rx:"2",stroke:m,strokeWidth:P}),_("rect",{x:"6.5",y:"7.5",width:"7",height:"1",rx:".5",fill:m,opacity:".25"})]});case"input":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"2",y:"4",width:"5.5",height:"1",rx:".5",fill:m,opacity:".25"}),_("rect",{x:"2",y:"6.5",width:"16",height:"5.5",rx:"1",stroke:m,strokeWidth:P}),_("rect",{x:"3.5",y:"8.5",width:"7",height:"1",rx:".5",fill:m,opacity:".12"})]});case"search":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"2",y:"4.5",width:"16",height:"7",rx:"3.5",stroke:m,strokeWidth:P}),_("circle",{cx:"6",cy:"8",r:"2",stroke:m,strokeWidth:P,opacity:".3"}),_("line",{x1:"7.5",y1:"9.5",x2:"9",y2:"11",stroke:m,strokeWidth:P,opacity:".3"}),_("rect",{x:"9.5",y:"7.5",width:"6",height:"1",rx:".5",fill:m,opacity:".12"})]});case"form":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"2",y:"1.5",width:"5.5",height:"1",rx:".5",fill:m,opacity:".25"}),_("rect",{x:"2",y:"3.5",width:"16",height:"3",rx:".75",stroke:m,strokeWidth:P}),_("rect",{x:"2",y:"8",width:"7",height:"1",rx:".5",fill:m,opacity:".25"}),_("rect",{x:"2",y:"10",width:"16",height:"3",rx:".75",stroke:m,strokeWidth:P}),_("rect",{x:"12",y:"14",width:"6",height:"2",rx:".75",stroke:m,strokeWidth:P})]});case"tabs":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"1",y:"5",width:"18",height:"10",rx:"1",stroke:m,strokeWidth:P}),_("rect",{x:"1",y:"2",width:"6",height:"3.5",rx:".75",stroke:m,strokeWidth:P}),_("rect",{x:"2.5",y:"3.25",width:"3",height:"1",rx:".5",fill:m,opacity:".25"}),_("rect",{x:"7",y:"2",width:"6",height:"3.5",rx:".75",stroke:m,strokeWidth:P})]});case"dropdown":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"2",y:"2",width:"16",height:"4",rx:"1",stroke:m,strokeWidth:P}),_("rect",{x:"3.5",y:"3.5",width:"7",height:"1",rx:".5",fill:m,opacity:".2"}),_("path",{d:"M15 3.5l1.5 1.5L18 3.5",stroke:m,strokeWidth:P,opacity:".3"}),_("rect",{x:"2",y:"7",width:"16",height:"7",rx:"1",stroke:m,strokeWidth:P,strokeDasharray:"2 1",opacity:".3"})]});case"toggle":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"4",y:"5",width:"12",height:"6",rx:"3",stroke:m,strokeWidth:P}),_("circle",{cx:"13",cy:"8",r:"2",fill:m,opacity:".3"})]});case"avatar":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("circle",{cx:"10",cy:"8",r:"6",stroke:m,strokeWidth:P}),_("circle",{cx:"10",cy:"6.5",r:"2",stroke:m,strokeWidth:P}),_("path",{d:"M6.5 13c0-2 1.5-3.5 3.5-3.5s3.5 1.5 3.5 3.5",stroke:m,strokeWidth:P})]});case"badge":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"3",y:"5",width:"14",height:"6",rx:"3",stroke:m,strokeWidth:P}),_("rect",{x:"6",y:"7.5",width:"8",height:"1",rx:".5",fill:m,opacity:".25"})]});case"breadcrumb":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"1.5",y:"7",width:"3.5",height:"1",rx:".5",fill:m,opacity:".3"}),_("path",{d:"M6.5 7l1 1-1 1",stroke:m,strokeWidth:P,opacity:".2"}),_("rect",{x:"9",y:"7",width:"3.5",height:"1",rx:".5",fill:m,opacity:".2"}),_("path",{d:"M14 7l1 1-1 1",stroke:m,strokeWidth:P,opacity:".2"}),_("rect",{x:"16.5",y:"7",width:"2",height:"1",rx:".5",fill:m,opacity:".15"})]});case"pagination":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"2",y:"5.5",width:"3.5",height:"5",rx:"1",stroke:m,strokeWidth:P}),_("rect",{x:"6.5",y:"5.5",width:"3.5",height:"5",rx:"1",stroke:m,strokeWidth:P}),_("rect",{x:"11",y:"5.5",width:"3.5",height:"5",rx:"1",fill:m,opacity:".15",stroke:m,strokeWidth:P}),_("rect",{x:"15.5",y:"5.5",width:"3.5",height:"5",rx:"1",stroke:m,strokeWidth:P})]});case"progress":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"2",y:"7",width:"16",height:"2",rx:"1",stroke:m,strokeWidth:P}),_("rect",{x:"2",y:"7",width:"10",height:"2",rx:"1",fill:m,opacity:".2"})]});case"toast":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"2",y:"4",width:"16",height:"8",rx:"1.5",stroke:m,strokeWidth:P}),_("circle",{cx:"5",cy:"8",r:"1.5",stroke:m,strokeWidth:P,opacity:".3"}),_("rect",{x:"8",y:"6.5",width:"7",height:"1",rx:".5",fill:m,opacity:".25"}),_("rect",{x:"8",y:"9",width:"5",height:"1",rx:".5",fill:m,opacity:".12"})]});case"tooltip":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"3",y:"3",width:"14",height:"7",rx:"1.5",stroke:m,strokeWidth:P}),_("rect",{x:"5.5",y:"5.5",width:"9",height:"1",rx:".5",fill:m,opacity:".25"}),_("path",{d:"M9 10l1 2.5 1-2.5",stroke:m,strokeWidth:P})]});case"pricing":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"2",y:"1",width:"16",height:"14",rx:"1.5",stroke:m,strokeWidth:P}),_("rect",{x:"6",y:"3",width:"8",height:"1.5",rx:".5",fill:m,opacity:".25"}),_("rect",{x:"7",y:"5.5",width:"6",height:"2",rx:".5",fill:m,opacity:".15"}),_("rect",{x:"5",y:"9",width:"10",height:"1",rx:".5",fill:m,opacity:".1"}),_("rect",{x:"5",y:"11",width:"10",height:"1",rx:".5",fill:m,opacity:".1"}),_("rect",{x:"6",y:"13",width:"8",height:"1.5",rx:".5",fill:m,opacity:".2"})]});case"testimonial":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"2",y:"1",width:"16",height:"14",rx:"1.5",stroke:m,strokeWidth:P}),_("text",{x:"4",y:"5.5",fontSize:"4",fill:m,opacity:".2",fontFamily:"serif",children:"\u201C"}),_("rect",{x:"4",y:"7",width:"12",height:"1",rx:".5",fill:m,opacity:".15"}),_("rect",{x:"4",y:"9",width:"9",height:"1",rx:".5",fill:m,opacity:".12"}),_("circle",{cx:"5.5",cy:"12.5",r:"1.5",stroke:m,strokeWidth:P,opacity:".25"}),_("rect",{x:"8",y:"12",width:"5",height:"1",rx:".5",fill:m,opacity:".15"})]});case"cta":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"1",y:"2",width:"18",height:"12",rx:"1",stroke:m,strokeWidth:P}),_("rect",{x:"5",y:"4.5",width:"10",height:"1.5",rx:".5",fill:m,opacity:".3"}),_("rect",{x:"6",y:"7.5",width:"8",height:"1",rx:".5",fill:m,opacity:".15"}),_("rect",{x:"7",y:"10",width:"6",height:"2.5",rx:"1",stroke:m,strokeWidth:P})]});case"alert":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"2",y:"4",width:"16",height:"8",rx:"1.5",stroke:m,strokeWidth:P}),_("circle",{cx:"6",cy:"8",r:"2",stroke:m,strokeWidth:P,opacity:".3"}),_("line",{x1:"6",y1:"7",x2:"6",y2:"8.5",stroke:m,strokeWidth:"0.6",opacity:".5"}),_("circle",{cx:"6",cy:"9.3",r:".3",fill:m,opacity:".5"}),_("rect",{x:"9.5",y:"7",width:"6",height:"1",rx:".5",fill:m,opacity:".2"})]});case"banner":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"1",y:"5",width:"18",height:"6",rx:"1",stroke:m,strokeWidth:P}),_("rect",{x:"4",y:"7.5",width:"8",height:"1",rx:".5",fill:m,opacity:".25"}),_("rect",{x:"14",y:"7",width:"3.5",height:"2",rx:".75",stroke:m,strokeWidth:P})]});case"stat":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"3",y:"2",width:"14",height:"12",rx:"1.5",stroke:m,strokeWidth:P}),_("rect",{x:"6",y:"4.5",width:"8",height:"1",rx:".5",fill:m,opacity:".15"}),_("rect",{x:"5",y:"7",width:"10",height:"2.5",rx:".5",fill:m,opacity:".3"}),_("rect",{x:"7",y:"11",width:"6",height:"1",rx:".5",fill:m,opacity:".12"})]});case"stepper":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("circle",{cx:"4",cy:"8",r:"2",fill:m,opacity:".2",stroke:m,strokeWidth:P}),_("line",{x1:"6",y1:"8",x2:"8",y2:"8",stroke:m,strokeWidth:".4",opacity:".3"}),_("circle",{cx:"10",cy:"8",r:"2",stroke:m,strokeWidth:P}),_("line",{x1:"12",y1:"8",x2:"14",y2:"8",stroke:m,strokeWidth:".4",opacity:".3"}),_("circle",{cx:"16",cy:"8",r:"2",stroke:m,strokeWidth:P})]});case"tag":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"3",y:"5",width:"14",height:"6",rx:"1.5",stroke:m,strokeWidth:P}),_("rect",{x:"5.5",y:"7.5",width:"6",height:"1",rx:".5",fill:m,opacity:".25"}),_("line",{x1:"14",y1:"6.5",x2:"15.5",y2:"9.5",stroke:m,strokeWidth:P,opacity:".2"}),_("line",{x1:"15.5",y1:"6.5",x2:"14",y2:"9.5",stroke:m,strokeWidth:P,opacity:".2"})]});case"rating":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("path",{d:"M4 5.5l1 2 2.2.3-1.6 1.5.4 2.2L4 10.3l-2 1.2.4-2.2L.8 7.8 3 7.5z",fill:m,opacity:".25"}),_("path",{d:"M10 5.5l1 2 2.2.3-1.6 1.5.4 2.2L10 10.3l-2 1.2.4-2.2L6.8 7.8 9 7.5z",fill:m,opacity:".25"}),_("path",{d:"M16 5.5l1 2 2.2.3-1.6 1.5.4 2.2L16 10.3l-2 1.2.4-2.2-1.6-1.5 2.2-.3z",stroke:m,strokeWidth:P,opacity:".25"})]});case"map":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"2",y:"2",width:"16",height:"12",rx:"1",stroke:m,strokeWidth:P}),_("line",{x1:"2",y1:"6",x2:"18",y2:"10",stroke:m,strokeWidth:".3",opacity:".15"}),_("line",{x1:"7",y1:"2",x2:"11",y2:"14",stroke:m,strokeWidth:".3",opacity:".15"}),_("path",{d:"M10 5c-1.7 0-3 1.3-3 3 0 2.5 3 5 3 5s3-2.5 3-5c0-1.7-1.3-3-3-3z",fill:m,opacity:".15",stroke:m,strokeWidth:P})]});case"timeline":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("line",{x1:"5",y1:"2",x2:"5",y2:"14",stroke:m,strokeWidth:".4",opacity:".25"}),_("circle",{cx:"5",cy:"4",r:"1.5",fill:m,opacity:".2",stroke:m,strokeWidth:P}),_("rect",{x:"8",y:"3",width:"8",height:"1",rx:".5",fill:m,opacity:".25"}),_("circle",{cx:"5",cy:"8.5",r:"1.5",stroke:m,strokeWidth:P}),_("rect",{x:"8",y:"7.5",width:"6",height:"1",rx:".5",fill:m,opacity:".15"}),_("circle",{cx:"5",cy:"13",r:"1.5",stroke:m,strokeWidth:P}),_("rect",{x:"8",y:"12",width:"7",height:"1",rx:".5",fill:m,opacity:".15"})]});case"fileUpload":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"3",y:"2",width:"14",height:"12",rx:"1.5",stroke:m,strokeWidth:P,strokeDasharray:"2 1"}),_("path",{d:"M10 10V5.5m0 0L7.5 8m2.5-2.5L12.5 8",stroke:m,strokeWidth:P,opacity:".3"}),_("rect",{x:"7",y:"11.5",width:"6",height:"1",rx:".5",fill:m,opacity:".15"})]});case"codeBlock":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"2",y:"2",width:"16",height:"12",rx:"1",stroke:m,strokeWidth:P}),_("circle",{cx:"4",cy:"4",r:".6",fill:m,opacity:".3"}),_("circle",{cx:"5.5",cy:"4",r:".6",fill:m,opacity:".3"}),_("circle",{cx:"7",cy:"4",r:".6",fill:m,opacity:".3"}),_("rect",{x:"4",y:"7",width:"7",height:"1",rx:".5",fill:m,opacity:".2"}),_("rect",{x:"6",y:"9",width:"5",height:"1",rx:".5",fill:m,opacity:".15"}),_("rect",{x:"4",y:"11",width:"8",height:"1",rx:".5",fill:m,opacity:".12"})]});case"calendar":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"2",y:"3",width:"16",height:"12",rx:"1",stroke:m,strokeWidth:P}),_("line",{x1:"2",y1:"6.5",x2:"18",y2:"6.5",stroke:m,strokeWidth:".4",opacity:".25"}),_("rect",{x:"5",y:"4",width:"1",height:"1.5",rx:".3",fill:m,opacity:".2"}),_("rect",{x:"14",y:"4",width:"1",height:"1.5",rx:".3",fill:m,opacity:".2"}),_("circle",{cx:"7",cy:"9",r:".6",fill:m,opacity:".2"}),_("circle",{cx:"10",cy:"9",r:".6",fill:m,opacity:".2"}),_("circle",{cx:"13",cy:"9",r:".6",fill:m,opacity:".3"}),_("circle",{cx:"7",cy:"12",r:".6",fill:m,opacity:".2"}),_("circle",{cx:"10",cy:"12",r:".6",fill:m,opacity:".2"})]});case"notification":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"2",y:"3",width:"16",height:"10",rx:"1.5",stroke:m,strokeWidth:P}),_("circle",{cx:"5.5",cy:"8",r:"2",stroke:m,strokeWidth:P,opacity:".25"}),_("rect",{x:"9",y:"6",width:"6",height:"1",rx:".5",fill:m,opacity:".25"}),_("rect",{x:"9",y:"8.5",width:"4.5",height:"1",rx:".5",fill:m,opacity:".12"}),_("circle",{cx:"16.5",cy:"4.5",r:"1.5",fill:m,opacity:".25"})]});case"productCard":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"3",y:"1",width:"14",height:"14",rx:"1.5",stroke:m,strokeWidth:P}),_("rect",{x:"3",y:"1",width:"14",height:"6",rx:"1",fill:m,opacity:".04"}),_("rect",{x:"5",y:"8.5",width:"7",height:"1",rx:".5",fill:m,opacity:".25"}),_("rect",{x:"5",y:"10.5",width:"4",height:"1.5",rx:".5",fill:m,opacity:".15"}),_("rect",{x:"12",y:"12",width:"4",height:"2",rx:".75",stroke:m,strokeWidth:P})]});case"profile":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("circle",{cx:"10",cy:"5",r:"3",stroke:m,strokeWidth:P}),_("rect",{x:"5",y:"10",width:"10",height:"1.5",rx:".5",fill:m,opacity:".25"}),_("rect",{x:"7",y:"12.5",width:"6",height:"1",rx:".5",fill:m,opacity:".12"})]});case"drawer":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"9",y:"1",width:"10",height:"14",rx:"1",stroke:m,strokeWidth:P}),_("rect",{x:"10.5",y:"4",width:"5",height:"1",rx:".5",fill:m,opacity:".25"}),_("rect",{x:"10.5",y:"6.5",width:"7",height:"1",rx:".5",fill:m,opacity:".15"}),_("rect",{x:"10.5",y:"9",width:"6",height:"1",rx:".5",fill:m,opacity:".15"}),_("rect",{x:"1",y:"1",width:"7",height:"14",rx:"1",stroke:m,strokeWidth:P,opacity:".15"})]});case"popover":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"3",y:"2",width:"14",height:"9",rx:"1.5",stroke:m,strokeWidth:P}),_("rect",{x:"5",y:"4.5",width:"8",height:"1",rx:".5",fill:m,opacity:".25"}),_("rect",{x:"5",y:"7",width:"6",height:"1",rx:".5",fill:m,opacity:".15"}),_("path",{d:"M9 11l1 2.5 1-2.5",stroke:m,strokeWidth:P})]});case"logo":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"2",y:"3",width:"10",height:"10",rx:"2",stroke:m,strokeWidth:P}),_("path",{d:"M5 9.5l2-4 2 4",stroke:m,strokeWidth:P,opacity:".3"}),_("rect",{x:"14",y:"6",width:"4",height:"1",rx:".5",fill:m,opacity:".2"}),_("rect",{x:"14",y:"8.5",width:"3",height:"1",rx:".5",fill:m,opacity:".12"})]});case"faq":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("text",{x:"2.5",y:"5.5",fontSize:"4",fill:m,opacity:".3",fontWeight:"bold",children:"?"}),_("rect",{x:"7",y:"3",width:"10",height:"1",rx:".5",fill:m,opacity:".25"}),_("rect",{x:"7",y:"5.5",width:"8",height:"1",rx:".5",fill:m,opacity:".12"}),_("text",{x:"2.5",y:"11.5",fontSize:"4",fill:m,opacity:".3",fontWeight:"bold",children:"?"}),_("rect",{x:"7",y:"9",width:"9",height:"1",rx:".5",fill:m,opacity:".25"}),_("rect",{x:"7",y:"11.5",width:"7",height:"1",rx:".5",fill:m,opacity:".12"})]});case"gallery":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"1.5",y:"1.5",width:"5",height:"5",rx:".75",stroke:m,strokeWidth:P}),_("rect",{x:"7.5",y:"1.5",width:"5",height:"5",rx:".75",stroke:m,strokeWidth:P}),_("rect",{x:"13.5",y:"1.5",width:"5",height:"5",rx:".75",stroke:m,strokeWidth:P}),_("rect",{x:"1.5",y:"9.5",width:"5",height:"5",rx:".75",stroke:m,strokeWidth:P}),_("rect",{x:"7.5",y:"9.5",width:"5",height:"5",rx:".75",stroke:m,strokeWidth:P}),_("rect",{x:"13.5",y:"9.5",width:"5",height:"5",rx:".75",stroke:m,strokeWidth:P})]});case"checkbox":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"5",y:"4",width:"8",height:"8",rx:"1.5",stroke:m,strokeWidth:P}),_("path",{d:"M7.5 8l1.5 1.5 3-3",stroke:m,strokeWidth:P,opacity:".35"})]});case"radio":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("circle",{cx:"10",cy:"8",r:"4",stroke:m,strokeWidth:P}),_("circle",{cx:"10",cy:"8",r:"2",fill:m,opacity:".3"})]});case"slider":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"2",y:"7.5",width:"16",height:"1",rx:".5",fill:m,opacity:".15"}),_("rect",{x:"2",y:"7.5",width:"10",height:"1",rx:".5",fill:m,opacity:".25"}),_("circle",{cx:"12",cy:"8",r:"2.5",stroke:m,strokeWidth:P})]});case"datePicker":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"2",y:"1",width:"16",height:"5",rx:"1",stroke:m,strokeWidth:P}),_("rect",{x:"3.5",y:"3",width:"5",height:"1",rx:".5",fill:m,opacity:".2"}),_("rect",{x:"14",y:"2.5",width:"2.5",height:"2",rx:".5",fill:m,opacity:".12"}),_("rect",{x:"2",y:"7",width:"16",height:"8",rx:"1",stroke:m,strokeWidth:P,strokeDasharray:"2 1",opacity:".3"}),_("circle",{cx:"6",cy:"10",r:".6",fill:m,opacity:".2"}),_("circle",{cx:"10",cy:"10",r:".6",fill:m,opacity:".3"}),_("circle",{cx:"14",cy:"10",r:".6",fill:m,opacity:".2"}),_("circle",{cx:"6",cy:"13",r:".6",fill:m,opacity:".2"}),_("circle",{cx:"10",cy:"13",r:".6",fill:m,opacity:".2"})]});case"skeleton":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"2",y:"2",width:"16",height:"3",rx:"1",fill:m,opacity:".08"}),_("rect",{x:"2",y:"7",width:"10",height:"2",rx:".75",fill:m,opacity:".08"}),_("rect",{x:"2",y:"11",width:"13",height:"2",rx:".75",fill:m,opacity:".08"})]});case"chip":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"1.5",y:"5",width:"10",height:"6",rx:"3",fill:m,opacity:".08",stroke:m,strokeWidth:P}),_("rect",{x:"4",y:"7.5",width:"4",height:"1",rx:".5",fill:m,opacity:".25"}),_("line",{x1:"9.5",y1:"6.5",x2:"10.5",y2:"9.5",stroke:m,strokeWidth:P,opacity:".2"}),_("line",{x1:"10.5",y1:"6.5",x2:"9.5",y2:"9.5",stroke:m,strokeWidth:P,opacity:".2"}),_("rect",{x:"13",y:"5",width:"5.5",height:"6",rx:"3",stroke:m,strokeWidth:P,opacity:".25"})]});case"icon":return _("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:_("path",{d:"M10 3l1.5 3 3.5.5-2.5 2.5.5 3.5L10 11l-3 1.5.5-3.5L5 6.5l3.5-.5z",stroke:m,strokeWidth:P,opacity:".3"})});case"spinner":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("circle",{cx:"10",cy:"8",r:"5",stroke:m,strokeWidth:P,opacity:".12"}),_("path",{d:"M10 3a5 5 0 0 1 5 5",stroke:m,strokeWidth:P,opacity:".35",strokeLinecap:"round"})]});case"feature":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"2",y:"2",width:"5",height:"5",rx:"1.5",stroke:m,strokeWidth:P}),_("path",{d:"M4.5 3.5v3m-1.5-1.5h3",stroke:m,strokeWidth:P,opacity:".25"}),_("rect",{x:"9",y:"2.5",width:"8",height:"1.5",rx:".5",fill:m,opacity:".25"}),_("rect",{x:"9",y:"5.5",width:"6",height:"1",rx:".5",fill:m,opacity:".12"}),_("rect",{x:"2",y:"10",width:"5",height:"5",rx:"1.5",stroke:m,strokeWidth:P}),_("rect",{x:"9",y:"10.5",width:"7",height:"1.5",rx:".5",fill:m,opacity:".25"}),_("rect",{x:"9",y:"13.5",width:"5",height:"1",rx:".5",fill:m,opacity:".12"})]});case"team":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("circle",{cx:"5",cy:"5",r:"2.5",stroke:m,strokeWidth:P}),_("rect",{x:"2.5",y:"9",width:"5",height:"1",rx:".5",fill:m,opacity:".2"}),_("circle",{cx:"15",cy:"5",r:"2.5",stroke:m,strokeWidth:P}),_("rect",{x:"12.5",y:"9",width:"5",height:"1",rx:".5",fill:m,opacity:".2"}),_("circle",{cx:"10",cy:"5",r:"2.5",stroke:m,strokeWidth:P,opacity:".5"}),_("rect",{x:"7.5",y:"9",width:"5",height:"1",rx:".5",fill:m,opacity:".15"}),_("rect",{x:"4",y:"12",width:"12",height:"1",rx:".5",fill:m,opacity:".1"})]});case"login":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"3",y:"1",width:"14",height:"14",rx:"1.5",stroke:m,strokeWidth:P}),_("rect",{x:"6",y:"3",width:"8",height:"1.5",rx:".5",fill:m,opacity:".25"}),_("rect",{x:"5",y:"5.5",width:"10",height:"3",rx:".75",stroke:m,strokeWidth:P}),_("rect",{x:"5",y:"9.5",width:"10",height:"3",rx:".75",stroke:m,strokeWidth:P}),_("rect",{x:"6.5",y:"13.5",width:"7",height:"2",rx:".75",fill:m,opacity:".2"})]});case"contact":return J("svg",{viewBox:"0 0 20 16",width:"20",height:"16",fill:"none",children:[_("rect",{x:"2",y:"1",width:"16",height:"14",rx:"1.5",stroke:m,strokeWidth:P}),_("rect",{x:"4",y:"3",width:"5",height:"1",rx:".5",fill:m,opacity:".2"}),_("rect",{x:"4",y:"5",width:"12",height:"2.5",rx:".75",stroke:m,strokeWidth:P}),_("rect",{x:"4",y:"8.5",width:"12",height:"4",rx:".75",stroke:m,strokeWidth:P}),_("rect",{x:"11",y:"13.5",width:"5",height:"1.5",rx:".5",fill:m,opacity:".2"})]});default:return null}}function ev({activeType:e,onSelect:t,onDragStart:n,scrollRef:o,fadeClass:r,blankCanvas:i}){return _("div",{ref:o,className:`${D.placeScroll} ${r||""}`,children:Ig.map(l=>J("div",{className:D.paletteSection,children:[_("div",{className:D.paletteSectionTitle,children:l.section}),l.items.map(s=>J("button",{type:"button","aria-pressed":e===s.type,className:`${D.paletteItem} ${e===s.type?D.active:""} ${i?D.wireframe:""}`,onClick:()=>t(s.type),onMouseDown:a=>{a.button===0&&n(s.type,a)},children:[_("span",{className:D.paletteItemIcon,"aria-hidden":"true",children:_(Zx,{type:s.type})}),_("span",{className:D.paletteItemLabel,children:s.label})]},s.type))]},l.section))})}function tv({value:e,suffix:t}){let[n,o]=(0,Gt.useState)(null),[r,i]=(0,Gt.useState)(t),[l,s]=(0,Gt.useState)("up"),a=(0,Gt.useRef)(e),c=(0,Gt.useRef)(t),f=(0,Gt.useRef)(),u=n!==null&&r!==t;return(0,Gt.useEffect)(()=>{if(e!==a.current){if(e===0){a.current=e,c.current=t,o(null);return}s(e>a.current?"up":"down"),o(a.current),i(c.current),a.current=e,c.current=t,clearTimeout(f.current),f.current=nt(()=>o(null),250)}else c.current=t},[e,t]),n===null?J(F0,{children:[e,t?` ${t}`:""]}):u?J("span",{className:D.rollingWrap,children:[J("span",{style:{visibility:"hidden"},children:[e," ",t]}),J("span",{className:`${D.rollingNum} ${l==="up"?D.exitUp:D.exitDown}`,children:[n," ",r]},`o${n}-${e}`),J("span",{className:`${D.rollingNum} ${l==="up"?D.enterUp:D.enterDown}`,children:[e," ",t]},`n${e}`)]}):J(F0,{children:[J("span",{className:D.rollingWrap,children:[_("span",{style:{visibility:"hidden"},children:e}),_("span",{className:`${D.rollingNum} ${l==="up"?D.exitUp:D.exitDown}`,children:n},`o${n}-${e}`),_("span",{className:`${D.rollingNum} ${l==="up"?D.enterUp:D.enterDown}`,children:e},`n${e}`)]}),t?` ${t}`:""]})}function nv({activeType:e,onSelect:t,isDarkMode:n,sectionCount:o,onDetectSections:r,visible:i,onExited:l,placementCount:s,onClearPlacements:a,onDragStart:c,blankCanvas:f,onBlankCanvasChange:u,wireframePurpose:x,onWireframePurposeChange:S,Tooltip:b}){let{ref:N,mounted:E}=Rg(i,{onExited:l}),[g,v]=(0,Gt.useState)(!1),[h,C]=(0,Gt.useState)(!0),U=(0,Gt.useRef)(0),V=(0,Gt.useRef)(""),B=(0,Gt.useRef)(null),[q,F]=(0,Gt.useState)(""),K=s>0||o>0,se=s+o;if(se>0&&(U.current=se,V.current=f?se===1?"Component":"Components":se===1?"Change":"Changes"),(0,Gt.useEffect)(()=>{if(K)g?C(!1):(C(!0),v(!0),Ec(()=>{Ec(()=>{C(!1)})}));else{C(!0);let fe=nt(()=>v(!1),300);return()=>clearTimeout(fe)}},[K]),(0,Gt.useEffect)(()=>{if(!i)return;let fe=B.current;if(!fe)return;let oe=()=>F(Jx(fe));fe.addEventListener("scroll",oe,{passive:!0});let ae=new ResizeObserver(oe);return ae.observe(fe),()=>{fe.removeEventListener("scroll",oe),ae.disconnect()}},[i]),!E)return null;let Z=[];return s>0&&Z.push("placed"),o>0&&Z.push("captured"),J("div",{className:`${D.palette} ${n?"":D.light}`,ref:fe=>{N.current=fe,fe?.toggleAttribute("inert",!i)},"aria-hidden":!i,"data-feedback-toolbar":!0,"data-agentation-palette":!0,onClick:fe=>fe.stopPropagation(),onMouseDown:fe=>fe.stopPropagation(),children:[J("div",{className:D.paletteHeader,children:[_("div",{className:D.paletteHeaderTitle,children:"Layout Mode"}),J("div",{className:D.paletteHeaderDesc,children:["Rearrange and resize existing elements, add new components, and explore layout ideas. Agent results may vary."," ",_("a",{href:"https://agentation.com/features#layout-mode",target:"_blank",rel:"noopener noreferrer",children:"Learn more."})]})]}),J("button",{type:"button","aria-pressed":f,className:`${D.canvasToggle} ${f?D.active:""}`,onClick:()=>u(!f),children:[_("span",{className:D.canvasToggleIcon,"aria-hidden":"true",children:J("svg",{viewBox:"0 0 14 14",width:"14",height:"14",fill:"none",children:[_("rect",{x:"1",y:"1",width:"12",height:"12",rx:"2",stroke:"currentColor",strokeWidth:"1"}),_("circle",{cx:"4.5",cy:"4.5",r:"0.8",fill:"currentColor",opacity:".6"}),_("circle",{cx:"7",cy:"4.5",r:"0.8",fill:"currentColor",opacity:".6"}),_("circle",{cx:"9.5",cy:"4.5",r:"0.8",fill:"currentColor",opacity:".6"}),_("circle",{cx:"4.5",cy:"7",r:"0.8",fill:"currentColor",opacity:".6"}),_("circle",{cx:"7",cy:"7",r:"0.8",fill:"currentColor",opacity:".6"}),_("circle",{cx:"9.5",cy:"7",r:"0.8",fill:"currentColor",opacity:".6"}),_("circle",{cx:"4.5",cy:"9.5",r:"0.8",fill:"currentColor",opacity:".6"}),_("circle",{cx:"7",cy:"9.5",r:"0.8",fill:"currentColor",opacity:".6"}),_("circle",{cx:"9.5",cy:"9.5",r:"0.8",fill:"currentColor",opacity:".6"})]})}),_("span",{className:D.canvasToggleLabel,children:"Wireframe New Page"})]}),_("div",{className:`${D.wireframePurposeWrap} ${f?"":D.collapsed}`,"aria-hidden":!f,ref:fe=>{fe?.toggleAttribute("inert",!f)},children:_("div",{className:D.wireframePurposeInner,children:_("textarea",{className:D.wireframePurposeInput,placeholder:"Describe this page to provide additional context for your agent.",value:x,onChange:fe=>S(fe.target.value),rows:2})})}),_(ev,{activeType:e,onSelect:t,onDragStart:c,scrollRef:B,fadeClass:q,blankCanvas:f}),g&&_("div",{className:`${D.paletteFooterWrap} ${h?D.footerHidden:""}`,children:_("div",{className:D.paletteFooterInner,children:_("div",{className:D.paletteFooterInnerContent,children:J("div",{className:D.paletteFooter,children:[_("span",{className:D.paletteFooterCount,children:_(tv,{value:U.current,suffix:V.current})}),_("button",{className:D.paletteFooterClear,onClick:a,children:"Clear"})]})})})})]})}var ov=new Set(["nav","header","main","section","article","footer","aside"]),j_={banner:"Header",navigation:"Navigation",main:"Main Content",contentinfo:"Footer",complementary:"Sidebar",region:"Section"},W0={nav:"Navigation",header:"Header",main:"Main Content",section:"Section",article:"Article",footer:"Footer",aside:"Sidebar"},rv=new Set(["script","style","noscript","link","meta"]),iv=40;function $g(e){let t=e;for(;t&&t!==document.body&&t!==document.documentElement;){let n=window.getComputedStyle(t).position;if(n==="fixed"||n==="sticky")return!0;t=t.parentElement}return!1}function li(e){let t=e.tagName.toLowerCase();if(["nav","header","footer","main"].includes(t)&&document.querySelectorAll(t).length===1)return t;if(e.id)return`#${CSS.escape(e.id)}`;if(e.className&&typeof e.className=="string"){let r=e.className.split(/\s+/).filter(i=>i.length>0).find(i=>i.length>2&&!/^[a-zA-Z0-9]{6,}$/.test(i)&&!/^[a-z]{1,2}$/.test(i));if(r){let i=`${t}.${CSS.escape(r)}`;if(document.querySelectorAll(i).length===1)return i}}let n=e.parentElement;if(n){let r=Array.from(n.children).indexOf(e)+1;return`${n===document.body?"body":li(n)} > ${t}:nth-child(${r})`}return t}function Lc(e){let t=e.tagName.toLowerCase(),n=e.getAttribute("aria-label");if(n)return n;let o=e.getAttribute("role");if(o&&j_[o])return j_[o];if(W0[t])return W0[t];let r=e.querySelector("h1, h2, h3, h4, h5, h6");if(r){let l=r.textContent?.trim();if(l&&l.length<=50)return l;if(l)return l.slice(0,47)+"..."}let{name:i}=Gi(e);return i.charAt(0).toUpperCase()+i.slice(1)}function Tg(e){let t=e.className;return typeof t!="string"||!t?null:t.split(/\s+/).map(o=>o.replace(/[_][a-zA-Z0-9]{5,}.*$/,"")).find(o=>o.length>2&&!/^[a-z]{1,2}$/.test(o))||null}function Pg(e){let t=e.textContent?.trim();if(!t)return null;let n=t.replace(/\s+/g," ");return n.length<=30?n:n.slice(0,30)+"\u2026"}function lv(){let e=document.querySelector("main")||document.body,t=Array.from(e.children),n=t;e!==document.body&&t.length<3&&(n=Array.from(document.body.children));let o=[];return n.forEach((r,i)=>{if(!(r instanceof HTMLElement))return;let l=r.tagName.toLowerCase();if(rv.has(l)||r.hasAttribute("data-feedback-toolbar")||r.closest("[data-feedback-toolbar]"))return;let s=window.getComputedStyle(r);if(s.display==="none"||s.visibility==="hidden")return;let a=r.getBoundingClientRect();if(a.height<iv)return;let c=ov.has(l),f=r.getAttribute("role")&&j_[r.getAttribute("role")],u=l==="div"&&a.height>=60;if(!c&&!f&&!u)return;let x=window.scrollY,S=$g(r),b={x:a.x,y:S?a.y:a.y+x,width:a.width,height:a.height};o.push({id:`rs-${Date.now()}-${Math.random().toString(36).slice(2,7)}`,label:Lc(r),tagName:l,selector:li(r),role:r.getAttribute("role"),className:Tg(r),textSnippet:Pg(r),originalRect:b,currentRect:{...b},originalIndex:i,isFixed:S})}),o}function sv(e){let t=window.scrollY,n=e.getBoundingClientRect(),o=$g(e),r={x:n.x,y:o?n.y:n.y+t,width:n.width,height:n.height},i=e.parentElement,l=0;return i&&(l=Array.from(i.children).indexOf(e)),{id:`rs-${Date.now()}-${Math.random().toString(36).slice(2,7)}`,label:Lc(e),tagName:e.tagName.toLowerCase(),selector:li(e),role:e.getAttribute("role"),className:Tg(e),textSnippet:Pg(e),originalRect:r,currentRect:{...r},originalIndex:l,isFixed:o}}var H0={bg:"rgba(59, 130, 246, 0.08)",border:"rgba(59, 130, 246, 0.5)",pill:"#3b82f6"},U0=["nw","n","ne","e","se","s","sw","w"],mc=24,Y0=16,gc=5;function Q0(e,t,n,o){let r=1/0,i=1/0,l=e.x,s=e.x+e.width,a=e.x+e.width/2,c=e.y,f=e.y+e.height,u=e.y+e.height/2,x=[];for(let B of t)n.has(B.id)||x.push(B.currentRect);o&&x.push(...o);for(let B of x){let q=B.x,F=B.x+B.width,K=B.x+B.width/2,se=B.y,Z=B.y+B.height,fe=B.y+B.height/2;for(let oe of[l,s,a])for(let ae of[q,F,K]){let pe=ae-oe;Math.abs(pe)<gc&&Math.abs(pe)<Math.abs(r)&&(r=pe)}for(let oe of[c,f,u])for(let ae of[se,Z,fe]){let pe=ae-oe;Math.abs(pe)<gc&&Math.abs(pe)<Math.abs(i)&&(i=pe)}}let S=Math.abs(r)<gc?r:0,b=Math.abs(i)<gc?i:0,N=[],E=new Set,g=l+S,v=s+S,h=a+S,C=c+b,U=f+b,V=u+b;for(let B of x){let q=B.x,F=B.x+B.width,K=B.x+B.width/2,se=B.y,Z=B.y+B.height,fe=B.y+B.height/2;for(let oe of[q,K,F])for(let ae of[g,h,v])if(Math.abs(ae-oe)<.5){let pe=`x:${Math.round(oe)}`;E.has(pe)||(E.add(pe),N.push({axis:"x",pos:oe}))}for(let oe of[se,fe,Z])for(let ae of[C,V,U])if(Math.abs(ae-oe)<.5){let pe=`y:${Math.round(oe)}`;E.has(pe)||(E.add(pe),N.push({axis:"y",pos:oe}))}}return{dx:S,dy:b,guides:N}}var av=new Set(["script","style","noscript","link","meta","br","hr"]);function V0(e){let t=e;for(;t&&t!==document.body&&t!==document.documentElement;){if(t.closest("[data-feedback-toolbar]"))return null;if(av.has(t.tagName.toLowerCase())){t=t.parentElement;continue}let n=t.getBoundingClientRect();if(n.width>=Y0&&n.height>=Y0)return t;t=t.parentElement}return null}function cv({rearrangeState:e,onChange:t,isDarkMode:n,exiting:o,className:r,blankCanvas:i,extraSnapRects:l,onSelectionChange:s,deselectSignal:a,onDragMove:c,onDragEnd:f,clearing:u}){let{sections:x}=e,S=(0,Se.useRef)(e);S.current=e;let[b,N]=(0,Se.useState)(new Set);(0,Se.useEffect)(()=>{u&&N(new Set)},[u]);let E=(0,Se.useRef)(a);(0,Se.useEffect)(()=>{a!==E.current&&(E.current=a,N(new Set))},[a]);let[g,v]=(0,Se.useState)(null),[h,C]=(0,Se.useState)(!1),U=(0,Se.useRef)(!1),V=(0,Se.useCallback)(L=>{let R=x.find(z=>z.id===L);R&&(U.current=!!R.note,v(L),C(!1))},[x]),B=(0,Se.useCallback)(()=>{g&&(C(!0),nt(()=>{v(null),C(!1)},150))},[g]),q=(0,Se.useCallback)(L=>{g&&(t({...e,sections:x.map(R=>R.id===g?{...R,note:L.trim()||void 0}:R)}),B())},[g,x,e,t,B]);(0,Se.useEffect)(()=>{o&&g&&B()},[o]);let[F,K]=(0,Se.useState)(new Set),se=(0,Se.useRef)(new Map),[Z,fe]=(0,Se.useState)(null),[oe,ae]=(0,Se.useState)(null),[pe,kt]=(0,Se.useState)([]),[Oe,qe]=(0,Se.useState)(0),Ae=(0,Se.useRef)(null),_t=(0,Se.useRef)(new Set),qt=(0,Se.useRef)(new Map),[wn,Re]=(0,Se.useState)(new Map),[Zn,Ro]=(0,Se.useState)(new Map),qo=(0,Se.useRef)(new Set),Ko=(0,Se.useRef)(new Map),$o=(0,Se.useRef)(s);$o.current=s;let To=(0,Se.useRef)(c);To.current=c;let Po=(0,Se.useRef)(f);Po.current=f,(0,Se.useEffect)(()=>{i&&N(new Set)},[i]);let[Bt,eo]=(0,Se.useState)(()=>!e.sections.some(L=>{let R=L.originalRect,z=L.currentRect;return Math.abs(R.x-z.x)>1||Math.abs(R.y-z.y)>1||Math.abs(R.width-z.width)>1||Math.abs(R.height-z.height)>1}));(0,Se.useEffect)(()=>{if(!Bt){let L=nt(()=>eo(!0),380);return()=>clearTimeout(L)}},[]);let mo=(0,Se.useRef)(new Set);(0,Se.useEffect)(()=>{mo.current=new Set(x.map(L=>L.selector))},[x]),(0,Se.useEffect)(()=>{let L=()=>qe(window.scrollY);return L(),window.addEventListener("scroll",L,{passive:!0}),window.addEventListener("resize",L,{passive:!0}),()=>{window.removeEventListener("scroll",L),window.removeEventListener("resize",L)}},[]),(0,Se.useEffect)(()=>{let L=R=>{if(Ae.current){fe(null);return}let z=document.elementFromPoint(R.clientX,R.clientY);if(!z){fe(null);return}if(z.closest("[data-feedback-toolbar]")){fe(null);return}if(z.closest("[data-design-placement]")){fe(null);return}if(z.closest("[data-annotation-popup]")){fe(null);return}let j=V0(z);if(!j){fe(null);return}for(let te of mo.current)try{let Y=document.querySelector(te);if(Y&&(Y===j||j.contains(Y))){fe(null);return}}catch{}let ee=j.getBoundingClientRect();fe({x:ee.x,y:ee.y,w:ee.width,h:ee.height})};return document.addEventListener("mousemove",L,{passive:!0}),()=>document.removeEventListener("mousemove",L)},[x]),(0,Se.useEffect)(()=>{let L=document.body.style.userSelect;return document.body.style.webkitUserSelect="none",document.body.style.userSelect="none",()=>{document.body.style.webkitUserSelect=L,document.body.style.userSelect=L}},[]),(0,Se.useEffect)(()=>{let L=R=>{if(Ae.current||R.button!==0)return;let z=R.composedPath()[0]??R.target;if(!z||z.closest("[data-feedback-toolbar]")||z.closest("[data-design-placement]")||z.closest("[data-annotation-popup]"))return;let j=V0(z),ee=!1;if(j)for(let Y of mo.current)try{let de=document.querySelector(Y);if(de&&(de===j||j.contains(de))){ee=!0;break}}catch{}let te=!!(R.shiftKey||R.metaKey||R.ctrlKey);if(j&&!ee){R.preventDefault(),R.stopPropagation();let Y=sv(j),de=[...x,Y],ke=[...e.originalOrder,Y.id];t({...e,sections:de,originalOrder:ke});let Te=new Set([Y.id]);N(Te),$o.current?.(Te,te),fe(null);let rt=R.clientX,st=R.clientY,Je={x:Y.currentRect.x,y:Y.currentRect.y},je=Y.originalRect,Ie=!1,gt=0,mt=0;Ae.current="move";let at=Ze=>{let ft=Ze.clientX-rt,ct=Ze.clientY-st;if(!Ie&&(Math.abs(ft)>2||Math.abs(ct)>2)&&(Ie=!0),!Ie)return;let ie={x:Je.x+ft,y:Je.y+ct,width:Y.currentRect.width,height:Y.currentRect.height},In=Q0(ie,de,new Set([Y.id]),l);kt(In.guides);let bn=ft+In.dx,rn=ct+In.dy;gt=bn,mt=rn;let go=H().querySelector(`[data-rearrange-section="${Y.id}"]`);go&&(go.style.transform=`translate(${bn}px, ${rn}px)`),Re(new Map([[Y.id,{x:Je.x+bn,y:Je.y+rn,width:Y.currentRect.width,height:Y.currentRect.height}]])),To.current?.(bn,rn)},Pe=()=>{window.removeEventListener("mousemove",at),window.removeEventListener("mouseup",Pe),Ae.current=null,kt([]),Re(new Map);let Ze=H().querySelector(`[data-rearrange-section="${Y.id}"]`);Ze&&(Ze.style.transform=""),Ie&&t({...e,sections:de.map(ft=>ft.id===Y.id?{...ft,currentRect:{...ft.currentRect,x:Math.max(0,Je.x+gt),y:Math.max(0,Je.y+mt)}}:ft),originalOrder:ke}),Po.current?.(gt,mt,Ie)};window.addEventListener("mousemove",at),window.addEventListener("mouseup",Pe)}else if(ee&&j){R.preventDefault();for(let Y of x)try{let de=document.querySelector(Y.selector);if(de&&de===j){let ke=new Set([Y.id]);N(ke),$o.current?.(ke,te);return}}catch{}te||(N(new Set),$o.current?.(new Set,!1))}else te||(N(new Set),$o.current?.(new Set,!1))};return document.addEventListener("mousedown",L,!0),()=>document.removeEventListener("mousedown",L,!0)},[x,e,t]),(0,Se.useEffect)(()=>{let L=R=>{let z=R.composedPath()[0]||R.target;if(!(z.tagName==="INPUT"||z.tagName==="TEXTAREA"||z.isContentEditable)){if((R.key==="Backspace"||R.key==="Delete")&&b.size>0){R.preventDefault();let j=new Set(b);K(ee=>{let te=new Set(ee);for(let Y of j)te.add(Y);return te}),N(new Set),nt(()=>{let ee=S.current;t({...ee,sections:ee.sections.filter(te=>!j.has(te.id)),originalOrder:ee.originalOrder.filter(te=>!j.has(te))}),K(te=>{let Y=new Set(te);for(let de of j)Y.delete(de);return Y})},180);return}if(["ArrowUp","ArrowDown","ArrowLeft","ArrowRight"].includes(R.key)&&b.size>0){R.preventDefault();let j=R.shiftKey?20:1,ee=R.key==="ArrowLeft"?-j:R.key==="ArrowRight"?j:0,te=R.key==="ArrowUp"?-j:R.key==="ArrowDown"?j:0;t({...e,sections:x.map(Y=>b.has(Y.id)?{...Y,currentRect:{...Y.currentRect,x:Math.max(0,Y.currentRect.x+ee),y:Math.max(0,Y.currentRect.y+te)}}:Y)});return}R.key==="Escape"&&b.size>0&&N(new Set)}};return document.addEventListener("keydown",L),()=>document.removeEventListener("keydown",L)},[b,x,e,t]);let Er=(0,Se.useCallback)((L,R)=>{if(L.button!==0)return;let z=L.target;if(z.closest(`.${D.handle}`)||z.closest(`.${D.deleteButton}`))return;L.preventDefault(),L.stopPropagation();let j;L.shiftKey||L.metaKey||L.ctrlKey?(j=new Set(b),j.has(R)?j.delete(R):j.add(R)):b.has(R)?j=new Set(b):j=new Set([R]),N(j),(j.size!==b.size||[...j].some(Ie=>!b.has(Ie)))&&$o.current?.(j,!!(L.shiftKey||L.metaKey||L.ctrlKey));let te=L.clientX,Y=L.clientY,de=new Map;for(let Ie of x)j.has(Ie.id)&&de.set(Ie.id,{x:Ie.currentRect.x,y:Ie.currentRect.y});Ae.current="move";let ke=!1,Te=0,rt=0,st=new Map;for(let Ie of x)if(j.has(Ie.id)){let gt=H().querySelector(`[data-rearrange-section="${Ie.id}"]`);st.set(Ie.id,{outlineEl:gt,curW:Ie.currentRect.width,curH:Ie.currentRect.height})}let Je=Ie=>{let gt=Ie.clientX-te,mt=Ie.clientY-Y;if(gt===0&&mt===0)return;ke=!0;let at=1/0,Pe=1/0,Ze=-1/0,ft=-1/0;for(let[rn,{curW:go,curH:Bo}]of st){let to=de.get(rn);if(!to)continue;let An=to.x+gt,tl=to.y+mt;at=Math.min(at,An),Pe=Math.min(Pe,tl),Ze=Math.max(Ze,An+go),ft=Math.max(ft,tl+Bo)}let ct=Q0({x:at,y:Pe,width:Ze-at,height:ft-Pe},x,j,l),ie=gt+ct.dx,In=mt+ct.dy;Te=ie,rt=In,kt(ct.guides);for(let[,{outlineEl:rn}]of st)rn&&(rn.style.transform=`translate(${ie}px, ${In}px)`);let bn=new Map;for(let[rn,{curW:go,curH:Bo}]of st){let to=de.get(rn);if(to){let An={x:Math.max(0,to.x+ie),y:Math.max(0,to.y+In),width:go,height:Bo};bn.set(rn,An)}}Re(bn),To.current?.(ie,In)},je=Ie=>{window.removeEventListener("mousemove",Je),window.removeEventListener("mouseup",je),Ae.current=null,kt([]),Re(new Map);for(let[,{outlineEl:gt}]of st)gt&&(gt.style.transform="");if(ke){let gt=Ie.clientX-te,mt=Ie.clientY-Y;if(Math.abs(gt)<5&&Math.abs(mt)<5)t({...e,sections:x.map(at=>{let Pe=de.get(at.id);return Pe?{...at,currentRect:{...at.currentRect,x:Pe.x,y:Pe.y}}:at})});else{t({...e,sections:x.map(at=>{let Pe=de.get(at.id);return Pe?{...at,currentRect:{...at.currentRect,x:Math.max(0,Pe.x+Te),y:Math.max(0,Pe.y+rt)}}:at})}),Po.current?.(Te,rt,!0);return}}Po.current?.(0,0,!1)};window.addEventListener("mousemove",Je),window.addEventListener("mouseup",je)},[b,x,e,t]),On=(0,Se.useCallback)((L,R,z)=>{L.preventDefault(),L.stopPropagation();let j=x.find(je=>je.id===R);if(!j)return;N(new Set([R])),Ae.current="resize";let ee=L.clientX,te=L.clientY,Y={...j.currentRect},de=j.originalRect,ke=Y.width/Y.height,Te={...Y},rt=H().querySelector(`[data-rearrange-section="${R}"]`),st=je=>{let Ie=je.clientX-ee,gt=je.clientY-te,mt=Y.x,at=Y.y,Pe=Y.width,Ze=Y.height;if(z.includes("e")&&(Pe=Math.max(mc,Y.width+Ie)),z.includes("w")&&(Pe=Math.max(mc,Y.width-Ie),mt=Y.x+Y.width-Pe),z.includes("s")&&(Ze=Math.max(mc,Y.height+gt)),z.includes("n")&&(Ze=Math.max(mc,Y.height-gt),at=Y.y+Y.height-Ze),je.shiftKey)if(z.length===2){let ct=Math.abs(Pe-Y.width),ie=Math.abs(Ze-Y.height);ct>ie?Ze=Pe/ke:Pe=Ze*ke,z.includes("w")&&(mt=Y.x+Y.width-Pe),z.includes("n")&&(at=Y.y+Y.height-Ze)}else z==="e"||z==="w"?Ze=Pe/ke:Pe=Ze*ke,z==="w"&&(mt=Y.x+Y.width-Pe),z==="n"&&(at=Y.y+Y.height-Ze);Te={x:mt,y:at,width:Pe,height:Ze},rt&&(rt.style.left=`${mt}px`,rt.style.top=`${at-Oe}px`,rt.style.width=`${Pe}px`,rt.style.height=`${Ze}px`),ae({x:je.clientX+12,y:je.clientY+12,text:`${Math.round(Pe)} \xD7 ${Math.round(Ze)}`}),Re(new Map([[R,Te]]))},Je=()=>{window.removeEventListener("mousemove",st),window.removeEventListener("mouseup",Je),ae(null),Ae.current=null,Re(new Map),t({...e,sections:x.map(je=>je.id===R?{...je,currentRect:Te}:je)})};window.addEventListener("mousemove",st),window.addEventListener("mouseup",Je)},[x,e,t,Oe]),Do=(0,Se.useCallback)(L=>{K(R=>{let z=new Set(R);return z.add(L),z}),N(R=>{let z=new Set(R);return z.delete(L),z}),nt(()=>{let R=S.current;t({...R,sections:R.sections.filter(z=>z.id!==L),originalOrder:R.originalOrder.filter(z=>z!==L)}),K(z=>{let j=new Set(z);return j.delete(L),j})},180)},[t]),G=L=>{let R=L.originalRect,z=L.currentRect;return Math.abs(R.x-z.x)>1||Math.abs(R.y-z.y)>1||Math.abs(R.width-z.width)>1||Math.abs(R.height-z.height)>1},he=L=>{let R=L.originalRect,z=L.currentRect;return Math.abs(R.x-z.x)>1||Math.abs(R.y-z.y)>1},Ne=L=>{let R=L.originalRect,z=L.currentRect;return Math.abs(R.width-z.width)>1||Math.abs(R.height-z.height)>1};for(let L of x)qt.current.has(L.id)||(he(L)?qt.current.set(L.id,"move"):Ne(L)&&qt.current.set(L.id,"resize"));for(let L of qt.current.keys())x.some(R=>R.id===L)||qt.current.delete(L);let Me=x.filter(L=>{try{if(F.has(L.id)||b.has(L.id))return!0;let R=document.querySelector(L.selector);if(!R)return!1;let z=R.getBoundingClientRect(),j=L.originalRect;return Math.abs(z.width-j.width)+Math.abs(z.height-j.height)<200}catch{return!1}}),be=Me.filter(L=>G(L)),Ke=Me.filter(L=>!G(L)),We=new Set(be.map(L=>L.id));for(let L of _t.current)We.has(L)||_t.current.delete(L);let Qe=[...We].sort().join(",");for(let L of be)Ko.current.set(L.id,{currentRect:L.currentRect,originalRect:L.originalRect,isFixed:L.isFixed});(0,Se.useEffect)(()=>{let L=qo.current;qo.current=We;let R=new Map;for(let z of L)if(!We.has(z)){if(!x.some(ee=>ee.id===z))continue;let j=Ko.current.get(z);j&&(R.set(z,{orig:j.originalRect,target:j.currentRect,isFixed:j.isFixed}),Ko.current.delete(z))}if(R.size>0){Ro(j=>{let ee=new Map(j);for(let[te,Y]of R)ee.set(te,Y);return ee});let z=nt(()=>{Ro(j=>{let ee=new Map(j);for(let te of R.keys())ee.delete(te);return ee})},250);return()=>clearTimeout(z)}},[Qe,x]);let Ye=(0,Se.useRef)(null),H=()=>Ye.current?.getRootNode()??document;return No(j0,{children:[No("div",{ref:Ye,className:`${D.rearrangeOverlay} ${n?"":D.light} ${o?D.overlayExiting:""}${r?` ${r}`:""}`,"data-feedback-toolbar":!0,children:[Z&&Yt("div",{className:D.hoverHighlight,style:{left:Z.x,top:Z.y,width:Z.w,height:Z.h}}),Ke.map(L=>{let R=L.currentRect,z=L.isFixed?R.y:R.y-Oe,j=H0,ee=b.has(L.id);return No("div",{"data-rearrange-section":L.id,className:`${D.sectionOutline} ${ee?D.selected:""} ${u||o||F.has(L.id)?D.exiting:""}`,style:{left:R.x,top:z,width:R.width,height:R.height,borderColor:j.border,backgroundColor:j.bg,...Bt?{}:{opacity:0,animation:"none",transition:"none"}},onMouseDown:te=>Er(te,L.id),onDoubleClick:()=>V(L.id),children:[Yt("span",{className:D.sectionLabel,style:{backgroundColor:j.pill},children:L.label}),Yt("span",{className:`${D.sectionAnnotation} ${L.note?D.annotationVisible:""}`,children:(L.note&&se.current.set(L.id,L.note),L.note||se.current.get(L.id)||"")}),No("span",{className:D.sectionDimensions,children:[Math.round(R.width)," \xD7 ",Math.round(R.height)]}),Yt("div",{className:D.deleteButton,onMouseDown:te=>te.stopPropagation(),onClick:()=>Do(L.id),children:"\u2715"}),U0.map(te=>Yt("div",{className:`${D.handle} ${D[`handle${te.charAt(0).toUpperCase()}${te.slice(1)}`]}`,onMouseDown:Y=>On(Y,L.id,te)},te))]},L.id)}),be.map(L=>{let R=L.currentRect,z=L.isFixed?R.y:R.y-Oe,j=b.has(L.id),ee=he(L),te=Ne(L);if(i&&!j)return null;let de=!_t.current.has(L.id);return de&&_t.current.add(L.id),No("div",{"data-rearrange-section":L.id,className:`${D.ghostOutline} ${j?D.selected:""} ${u||o||F.has(L.id)?D.exiting:""}`,style:{left:R.x,top:z,width:R.width,height:R.height,...Bt?{}:{opacity:0,animation:"none",transition:"none"},...de?{}:{animation:"none"}},onMouseDown:ke=>Er(ke,L.id),onDoubleClick:()=>V(L.id),children:[Yt("span",{className:D.sectionLabel,style:{backgroundColor:H0.pill},children:L.label}),Yt("span",{className:`${D.sectionAnnotation} ${L.note?D.annotationVisible:""}`,children:(L.note&&se.current.set(L.id,L.note),L.note||se.current.get(L.id)||"")}),No("span",{className:D.sectionDimensions,children:[Math.round(R.width)," \xD7 ",Math.round(R.height)]}),Yt("div",{className:D.deleteButton,onMouseDown:ke=>ke.stopPropagation(),onClick:()=>Do(L.id),children:"\u2715"}),U0.map(ke=>Yt("div",{className:`${D.handle} ${D[`handle${ke.charAt(0).toUpperCase()}${ke.slice(1)}`]}`,onMouseDown:Te=>On(Te,L.id,ke)},ke)),Yt("span",{className:D.ghostBadge,children:(()=>{let ke=qt.current.get(L.id);if(ee&&te){let[Te,rt]=ke==="resize"?["Resize","Move"]:["Move","Resize"];return No(j0,{children:["Suggested ",Te," ",No("span",{className:D.ghostBadgeExtra,children:["& ",rt]})]})}return`Suggested ${te?"Resize":"Move"}`})()})]},L.id)})]}),!i&&(()=>{let L=[];for(let R of be){let z=wn.get(R.id);L.push({id:R.id,orig:R.originalRect,target:z||R.currentRect,isFixed:R.isFixed,isSelected:b.has(R.id),isExiting:F.has(R.id)})}for(let[R,z]of wn)if(!L.some(j=>j.id===R)){let j=x.find(ee=>ee.id===R);j&&L.push({id:R,orig:j.originalRect,target:z,isFixed:j.isFixed,isSelected:b.has(R)})}for(let[R,z]of Zn)L.some(j=>j.id===R)||L.push({id:R,orig:z.orig,target:z.target,isFixed:z.isFixed,isSelected:!1,isExiting:!0});return L.length===0?null:No("svg",{className:`${D.connectorSvg} ${u||o?D.connectorExiting:""}`,children:[L.map(({id:R,orig:z,target:j,isFixed:ee,isSelected:te,isExiting:Y})=>{let de=z.x+z.width/2,ke=(ee?z.y:z.y-Oe)+z.height/2,Te=j.x+j.width/2,rt=(ee?j.y:j.y-Oe)+j.height/2,st=Te-de,Je=rt-ke,je=Math.sqrt(st*st+Je*Je);if(je<2)return null;let Ie=Math.min(1,je/40),gt=Math.min(je*.3,60),mt=je>0?-Je/je:0,at=je>0?st/je:0,Pe=(de+Te)/2+mt*gt,Ze=(ke+rt)/2+at*gt,ft=wn.has(R),ct=ft||te?1:.4,ie=ft||te?1:.5;return No("g",{className:Y?D.connectorExiting:"",children:[Yt("path",{className:D.connectorLine,d:`M ${de} ${ke} Q ${Pe} ${Ze} ${Te} ${rt}`,fill:"none",stroke:"rgba(59, 130, 246, 0.45)",strokeWidth:"1.5",opacity:ct*Ie}),Yt("circle",{className:D.connectorDot,cx:de,cy:ke,r:4*Ie,fill:"rgba(59, 130, 246, 0.8)",stroke:"#fff",strokeWidth:"1.5",opacity:ie*Ie,filter:"url(#connDotShadow)"}),Yt("circle",{className:D.connectorDot,cx:Te,cy:rt,r:4*Ie,fill:"rgba(59, 130, 246, 0.8)",stroke:"#fff",strokeWidth:"1.5",opacity:ie*Ie,filter:"url(#connDotShadow)"})]},`conn-${R}`)}),Yt("defs",{children:Yt("filter",{id:"connDotShadow",x:"-50%",y:"-50%",width:"200%",height:"200%",children:Yt("feDropShadow",{dx:"0",dy:"0.5",stdDeviation:"1",floodOpacity:"0.15"})})})]})})(),g&&(()=>{let L=x.find(rt=>rt.id===g);if(!L)return null;let R=L.currentRect,z=L.isFixed?R.y:R.y-Oe,j=R.x+R.width/2,ee=z-8,te=z+R.height+8,Y=ee>200,de=te<window.innerHeight-100,ke=Math.max(160,Math.min(window.innerWidth-160,j)),Te;return Y?Te={left:ke,bottom:window.innerHeight-ee}:de?Te={left:ke,top:te}:Te={left:ke,top:Math.max(80,window.innerHeight/2-80)},Yt(J_,{element:L.label,placeholder:"Add a note about this section",initialValue:L.note??"",submitLabel:U.current?"Save":"Set",onSubmit:q,onCancel:B,onDelete:U.current?()=>{q("")}:void 0,isExiting:h,lightMode:!n,style:Te})})(),oe&&Yt("div",{className:D.sizeIndicator,style:{left:oe.x,top:oe.y},"data-feedback-toolbar":!0,children:oe.text}),pe.map((L,R)=>Yt("div",{className:D.guideLine,style:L.axis==="x"?{position:"fixed",left:L.pos,top:0,width:1,height:"100vh"}:{position:"fixed",left:0,top:L.pos-Oe,width:"100vw",height:1}},`${L.axis}-${L.pos}-${R}`))]})}var H_=new Set(["script","style","noscript","link","meta","br","hr"]);function dv(){let e=document.querySelector("main")||document.body,t=[],n=Array.from(e.children),o=e!==document.body&&n.length<3?Array.from(document.body.children):n;for(let r of o){if(!(r instanceof HTMLElement)||H_.has(r.tagName.toLowerCase())||r.hasAttribute("data-feedback-toolbar"))continue;let i=window.getComputedStyle(r);if(i.display==="none"||i.visibility==="hidden")continue;let l=r.getBoundingClientRect();if(!(l.height<10||l.width<10)){t.push({label:Lc(r),selector:li(r),top:l.top,bottom:l.bottom,left:l.left,right:l.right,area:l.width*l.height});for(let s of Array.from(r.children)){if(!(s instanceof HTMLElement)||H_.has(s.tagName.toLowerCase())||s.hasAttribute("data-feedback-toolbar"))continue;let a=window.getComputedStyle(s);if(a.display==="none"||a.visibility==="hidden")continue;let c=s.getBoundingClientRect();c.height<10||c.width<10||t.push({label:Lc(s),selector:li(s),top:c.top,bottom:c.bottom,left:c.left,right:c.right,area:c.width*c.height})}}}return t}function uv(e){let t=window.scrollY;return e.map(({label:n,selector:o,rect:r})=>{let i=r.y-t;return{label:n,selector:o,top:i,bottom:i+r.height,left:r.x,right:r.x+r.width,area:r.width*r.height}})}function _v(e){let t=window.scrollY,n=e.y-t,o=e.x;return{top:n,bottom:n+e.height,left:o,right:o+e.width,area:e.width*e.height}}function U_(e,t){let n=t?uv(t):dv(),o=_v(e),r=null,i=null,l=null,s=null,a=null;for(let b of n){if(Math.abs(b.left-o.left)<2&&Math.abs(b.top-o.top)<2&&Math.abs(b.right-b.left-e.width)<2&&Math.abs(b.bottom-b.top-e.height)<2)continue;b.left<=o.left+2&&b.right>=o.right-2&&b.top<=o.top+2&&b.bottom>=o.bottom-2&&b.area>o.area*1.5&&(!a||b.area<a._area)&&(a={label:b.label,selector:b.selector,_area:b.area});let N=o.right>b.left+5&&o.left<b.right-5,E=o.bottom>b.top+5&&o.top<b.bottom-5;if(N&&b.bottom<=o.top+5){let g=Math.round(o.top-b.bottom);(!r||g<r._dist)&&(r={label:b.label,selector:b.selector,gap:Math.max(0,g),_dist:g})}if(N&&b.top>=o.bottom-5){let g=Math.round(b.top-o.bottom);(!i||g<i._dist)&&(i={label:b.label,selector:b.selector,gap:Math.max(0,g),_dist:g})}if(E&&b.right<=o.left+5){let g=Math.round(o.left-b.right);(!l||g<l._dist)&&(l={label:b.label,selector:b.selector,gap:Math.max(0,g),_dist:g})}if(E&&b.left>=o.right-5){let g=Math.round(b.left-o.right);(!s||g<s._dist)&&(s={label:b.label,selector:b.selector,gap:Math.max(0,g),_dist:g})}}let c=window.innerWidth,f=window.innerHeight,u=hv(e,c),x=b=>b?{label:b.label,selector:b.selector,gap:b.gap}:null,S=fv(o,e,c,f,a?{label:a.label,selector:a.selector,_area:a._area}:null,n);return{above:x(r),below:x(i),left:x(l),right:x(s),alignment:u,containedIn:a?{label:a.label,selector:a.selector}:null,outOfBounds:S}}function fv(e,t,n,o,r,i){let l={},s=!1,a=[];if(e.left<-2&&a.push("left"),e.right>n+2&&a.push("right"),e.top<-2&&a.push("top"),e.bottom>o+2&&a.push("bottom"),a.length>0&&(l.viewport=a,s=!0),r){let c=i.find(f=>f.label===r.label&&f.selector===r.selector&&Math.abs(f.area-r._area)<10);if(c){let f=[];e.left<c.left-2&&f.push("left"),e.right>c.right+2&&f.push("right"),e.top<c.top-2&&f.push("top"),e.bottom>c.bottom+2&&f.push("bottom"),f.length>0&&(l.container={label:r.label,edges:f},s=!0)}}return s?l:null}function hv(e,t){if(e.width/t>.85)return"full-width";let o=e.x+e.width/2,r=t/2,i=o-r,l=t*.08;return Math.abs(i)<l?"center":i<0?"left":"right"}function Dg(e){switch(e){case"full-width":return"full-width";case"center":return"centered";case"left":return"left-aligned";case"right":return"right-aligned"}}function Bg(e,t={}){let n=[];e.above&&n.push(`Below \`${e.above.label}\`${e.above.gap>0?` (${e.above.gap}px gap)`:""}`),e.below&&n.push(`Above \`${e.below.label}\`${e.below.gap>0?` (${e.below.gap}px gap)`:""}`),t.includeLeftRight&&(e.left&&n.push(`Right of \`${e.left.label}\`${e.left.gap>0?` (${e.left.gap}px gap)`:""}`),e.right&&n.push(`Left of \`${e.right.label}\`${e.right.gap>0?` (${e.right.gap}px gap)`:""}`));let o=Dg(e.alignment);return e.containedIn?n.push(`${o.charAt(0).toUpperCase()+o.slice(1)} in \`${e.containedIn.label}\``):n.push(`${o.charAt(0).toUpperCase()+o.slice(1)} in page`),t.includePixelRef&&t.pixelRef&&n.push(`Pixel ref: \`${t.pixelRef}\``),e.outOfBounds&&(e.outOfBounds.viewport&&n.push(`**Outside viewport** (${e.outOfBounds.viewport.join(", ")} edge${e.outOfBounds.viewport.length>1?"s":""})`),e.outOfBounds.container&&n.push(`**Outside \`${e.outOfBounds.container.label}\`** (${e.outOfBounds.container.edges.join(", ")} edge${e.outOfBounds.container.edges.length>1?"s":""})`)),n}function pv(e,t,n){let o=[];e.above&&o.push(`below \`${e.above.label}\``),e.below&&o.push(`above \`${e.below.label}\``),e.left&&o.push(`right of \`${e.left.label}\``),e.right&&o.push(`left of \`${e.right.label}\``),e.containedIn&&o.push(`inside \`${e.containedIn.label}\``),o.push(Dg(e.alignment)),e.outOfBounds?.viewport&&o.push(`**outside viewport** (${e.outOfBounds.viewport.join(", ")})`),e.outOfBounds?.container&&o.push(`**outside \`${e.outOfBounds.container.label}\`** (${e.outOfBounds.container.edges.join(", ")})`);let r=n?`, ${Math.round(n.width)}\xD7${Math.round(n.height)}px`:"";return`at (${Math.round(t.x)}, ${Math.round(t.y)})${r}: ${o.join(", ")}`}var X0=15;function G0(e){if(e.length<2)return[];let t=[],n=new Set;for(let o=0;o<e.length;o++){if(n.has(o))continue;let r=[o];for(let i=o+1;i<e.length;i++)n.has(i)||Math.abs(e[o].rect.y-e[i].rect.y)<X0&&r.push(i);if(r.length>=2){let i=r.map(a=>e[a]);i.sort((a,c)=>a.rect.x-c.rect.x);let l=[];for(let a=0;a<i.length-1;a++)l.push(Math.round(i[a+1].rect.x-(i[a].rect.x+i[a].rect.width)));let s=Math.round(i.reduce((a,c)=>a+c.rect.y,0)/i.length);t.push({labels:i.map(a=>a.label),type:"row",sharedEdge:s,gaps:l,avgGap:l.length?Math.round(l.reduce((a,c)=>a+c,0)/l.length):0}),r.forEach(a=>n.add(a))}}for(let o=0;o<e.length;o++){if(n.has(o))continue;let r=[o];for(let i=o+1;i<e.length;i++)n.has(i)||Math.abs(e[o].rect.x-e[i].rect.x)<X0&&r.push(i);if(r.length>=2){let i=r.map(a=>e[a]);i.sort((a,c)=>a.rect.y-c.rect.y);let l=[];for(let a=0;a<i.length-1;a++)l.push(Math.round(i[a+1].rect.y-(i[a].rect.y+i[a].rect.height)));let s=Math.round(i.reduce((a,c)=>a+c.rect.x,0)/i.length);t.push({labels:i.map(a=>a.label),type:"column",sharedEdge:s,gaps:l,avgGap:l.length?Math.round(l.reduce((a,c)=>a+c,0)/l.length):0}),r.forEach(a=>n.add(a))}}return t}function mv(e){if(e.length<2)return[];let t=G0(e.map(l=>({label:l.label,rect:l.originalRect}))),n=G0(e.map(l=>({label:l.label,rect:l.currentRect}))),o=[],r=new Set;for(let l of t){let s=new Set(l.labels),a=null,c=0;for(let f of n){let u=f.labels.filter(x=>s.has(x)).length;u>=2&&u>c&&(a=f,c=u)}if(a){let f=a.labels.filter(x=>s.has(x)),u=f.join(", ");if(a.type!==l.type){let x=l.type==="row"?"y":"x",S=a.type==="row"?"y":"x";o.push(`**${u}**: ${l.type} (${x}\u2248${l.sharedEdge}, ${l.avgGap}px gaps) \u2192 ${a.type} (${S}\u2248${a.sharedEdge}, ${a.avgGap}px gaps)`)}else if(Math.abs(l.sharedEdge-a.sharedEdge)>20||Math.abs(l.avgGap-a.avgGap)>5){let x=l.type==="row"?"y":"x",S=Math.abs(l.sharedEdge-a.sharedEdge)>20?` ${x}: ${l.sharedEdge} \u2192 ${a.sharedEdge}`:"",b=Math.abs(l.avgGap-a.avgGap)>5?` gaps: ${l.avgGap}px \u2192 ${a.avgGap}px`:"";o.push(`**${u}**: ${l.type} shifted \u2014${S}${b}`)}f.forEach(x=>r.add(x))}else{let f=l.labels.join(", "),u=l.type==="row"?"y":"x";o.push(`**${f}**: ${l.type} (${u}\u2248${l.sharedEdge}) dissolved`),l.labels.forEach(x=>r.add(x))}}for(let l of n){if(l.labels.every(c=>r.has(c))||l.labels.filter(c=>!r.has(c)).length<2)continue;if(!t.some(c=>c.labels.filter(u=>l.labels.includes(u)).length>=2)){let c=l.type==="row"?"y":"x";o.push(`**${l.labels.join(", ")}**: new ${l.type} (${c}\u2248${l.sharedEdge}, ${l.avgGap}px gaps)`),l.labels.forEach(f=>r.add(f))}}let i=e.filter(l=>!r.has(l.label));if(i.length>=2){let l={};for(let s of i){let a=Math.round(s.currentRect.x/5)*5;(l[a]??(l[a]=[])).push(s.label)}for(let[s,a]of Object.entries(l))a.length>=2&&o.push(`**${a.join(", ")}**: shared left edge at x\u2248${s}`)}return o}function zg(e){if(typeof document>"u")return{viewport:e,contentArea:null};let t=[],n=new Set,o=s=>{n.has(s)||s instanceof HTMLElement&&(s.hasAttribute("data-feedback-toolbar")||H_.has(s.tagName.toLowerCase())||(n.add(s),t.push(s)))},r=document.querySelector("main");r&&o(r);let i=document.querySelector("[role='main']");i&&o(i);for(let s of Array.from(document.body.children))if(o(s),s.children){for(let a of Array.from(s.children))if(o(a),a.children)for(let c of Array.from(a.children))o(c)}let l=null;for(let s of t){let a=s.getBoundingClientRect();if(a.height<50)continue;let c=getComputedStyle(s);if(c.maxWidth&&c.maxWidth!=="none"&&c.maxWidth!=="0px"){(!l||a.width<l.rect.width)&&(l={el:s,rect:a});continue}!l&&a.width<e.width-20&&a.width>100&&(l={el:s,rect:a})}if(l){let{el:s,rect:a}=l;return{viewport:e,contentArea:{width:Math.round(a.width),left:Math.round(a.left),right:Math.round(a.right),centerX:Math.round(a.left+a.width/2),selector:li(s)}}}return{viewport:e,contentArea:null}}function gv(e){if(typeof document>"u")return null;let t=document.querySelector(e);if(!t?.parentElement)return null;let n=getComputedStyle(t.parentElement),o={parentDisplay:n.display,parentSelector:li(t.parentElement)};return n.display.includes("flex")&&(o.flexDirection=n.flexDirection),n.display.includes("grid")&&n.gridTemplateColumns!=="none"&&(o.gridCols=n.gridTemplateColumns),n.gap&&n.gap!=="normal"&&n.gap!=="0px"&&(o.gap=n.gap),o}function Og(e,t){let n=t.contentArea,o=n?n.width:t.viewport.width,r=n?n.left:0,i=n?n.centerX:Math.round(t.viewport.width/2),l=Math.round(e.x-r),s=Math.round(r+o-(e.x+e.width)),a=(e.width/o*100).toFixed(1),c=e.x+e.width/2,f=Math.abs(c-i)<20,u=e.width/o>.95,x=[];return u?x.push("`width: 100%` of container"):x.push(`left \`${l}px\` in container, right \`${s}px\`, width \`${a}%\` (\`${Math.round(e.width)}px\`)`),f&&!u&&x.push("centered \u2014 `margin-inline: auto`"),x.join(" \u2014 ")}function Ag(e){let{viewport:t,contentArea:n}=e,o=`### Reference Frame
`;if(o+=`- Viewport: \`${t.width}\xD7${t.height}px\`
`,n){let r=n;o+=`- Content area: \`${r.width}px\` wide, left edge at \`x=${r.left}\`, right at \`x=${r.right}\` (\`${r.selector}\`)
`,o+=`- Pixel \u2192 CSS translation:
`,o+=`  - **Horizontal position in container**: \`element.x - ${r.left}\` \u2192 use as \`margin-left\` or \`left\`
`,o+=`  - **Width as % of container**: \`element.width / ${r.width} \xD7 100\` \u2192 use as \`width: X%\`
`,o+="  - **Vertical gap between elements**: `nextElement.y - (prevElement.y + prevElement.height)` \u2192 use as `margin-top` or `gap`\n",o+=`  - **Centered**: if \`|element.centerX - ${r.centerX}| < 20px\` \u2192 use \`margin-inline: auto\`
`}else o+=`- No distinct content container \u2014 elements positioned relative to full viewport
`,o+=`- Pixel \u2192 CSS translation:
`,o+=`  - **Width as % of viewport**: \`element.width / ${t.width} \xD7 100\` \u2192 use as \`width: X%\`
`,o+=`  - **Centered**: if \`|(element.x + element.width/2) - ${Math.round(t.width/2)}| < 20px\` \u2192 use \`margin-inline: auto\`
`;return o+=`
`,o}function yv(e){let t=gv(e);if(!t)return null;let n=`\`${t.parentDisplay}\``;return t.flexDirection&&(n+=`, flex-direction: \`${t.flexDirection}\``),t.gridCols&&(n+=`, grid-template-columns: \`${t.gridCols}\``),t.gap&&(n+=`, gap: \`${t.gap}\``),`Parent: ${n} (\`${t.parentSelector}\`)`}function q0(e,t,n,o="standard"){if(e.length===0)return"";let r=[...e].sort((E,g)=>Math.abs(E.y-g.y)<20?E.x-g.x:E.y-g.y),i="";if(n?.blankCanvas?(i+=`## Wireframe: New Page

`,n.wireframePurpose&&(i+=`> **Purpose:** ${n.wireframePurpose}
>
`),i+=`> ${e.length} component${e.length!==1?"s":""} placed \u2014 this is a standalone wireframe, not related to the current page.
>
> This wireframe is a rough sketch for exploring ideas.

`):i+=`## Design Layout

> ${e.length} component${e.length!==1?"s":""} placed

`,o==="compact")return i+=`### Components
`,r.forEach((E,g)=>{let v=po[E.type]?.label||E.type;i+=`${g+1}. **${v}** \u2014 \`${Math.round(E.width)}\xD7${Math.round(E.height)}px\` at \`(${Math.round(E.x)}, ${Math.round(E.y)})\`
`,E.text&&(i+=`   - Note: "${E.text}"
`)}),i;let l=zg(t);i+=Ag(l),i+=`### Components
`,r.forEach((E,g)=>{let v=po[E.type]?.label||E.type,h={x:E.x,y:E.y,width:E.width,height:E.height};i+=`${g+1}. **${v}** \u2014 \`${Math.round(E.width)}\xD7${Math.round(E.height)}px\` at \`(${Math.round(E.x)}, ${Math.round(E.y)})\`
`,E.text&&(i+=`   - Note: "${E.text}"
`);let C=U_(h),V=Bg(C,{includeLeftRight:o==="detailed"||o==="forensic"});for(let q of V)i+=`   - ${q}
`;let B=Og(h,l);B&&(i+=`   - CSS: ${B}
`)}),i+=`
### Layout Analysis
`;let s=[];for(let E of r){let g=s.find(v=>Math.abs(v.y-E.y)<30);g?g.items.push(E):s.push({y:E.y,items:[E]})}if(s.sort((E,g)=>E.y-g.y),s.forEach((E,g)=>{E.items.sort((h,C)=>h.x-C.x);let v=E.items.map(h=>po[h.type]?.label||h.type);if(E.items.length===1){let C=E.items[0].width>t.width*.8;i+=`- Row ${g+1} (y\u2248${Math.round(E.y)}): ${v[0]}${C?" \u2014 full width":""}
`}else i+=`- Row ${g+1} (y\u2248${Math.round(E.y)}): ${v.join(" | ")} \u2014 ${E.items.length} items side by side
`}),o==="detailed"||o==="forensic"){i+=`
### Spacing & Gaps
`;for(let E=0;E<r.length-1;E++){let g=r[E],v=r[E+1],h=po[g.type]?.label||g.type,C=po[v.type]?.label||v.type,U=Math.round(v.y-(g.y+g.height)),V=Math.round(v.x-(g.x+g.width));Math.abs(g.y-v.y)<30?i+=`- ${h} \u2192 ${C}: \`${V}px\` horizontal gap
`:i+=`- ${h} \u2192 ${C}: \`${U}px\` vertical gap
`}if(o==="forensic"&&r.length>2){i+=`
### All Pairwise Gaps
`;for(let E=0;E<r.length;E++)for(let g=E+1;g<r.length;g++){let v=r[E],h=r[g],C=po[v.type]?.label||v.type,U=po[h.type]?.label||h.type,V=Math.round(h.y-(v.y+v.height)),B=Math.round(h.x-(v.x+v.width));i+=`- ${C} \u2194 ${U}: h=\`${B}px\` v=\`${V}px\`
`}}o==="forensic"&&(i+=`
### Z-Order (placement order)
`,e.forEach((E,g)=>{let v=po[E.type]?.label||E.type;i+=`${g}. ${v} at \`(${Math.round(E.x)}, ${Math.round(E.y)})\`
`}))}i+=`
### Suggested Implementation
`;let a=r.some(E=>E.type==="navigation"),c=r.some(E=>E.type==="hero"),f=r.some(E=>E.type==="sidebar"),u=r.some(E=>E.type==="footer"),x=r.filter(E=>E.type==="card"),S=r.filter(E=>E.type==="form"),b=r.filter(E=>E.type==="table"),N=r.filter(E=>E.type==="modal");if(a&&(i+=`- Top navigation bar with logo + nav links + CTA
`),c&&(i+=`- Hero section with heading, subtext, and call-to-action
`),f&&(i+=`- Sidebar layout \u2014 use CSS Grid with sidebar + main content area
`),x.length>1?i+=`- ${x.length}-column card grid \u2014 use CSS Grid or Flexbox
`:x.length===1&&(i+=`- Card component with image + content area
`),S.length>0&&(i+=`- ${S.length} form${S.length>1?"s":""} \u2014 add proper labels, validation, and submit handling
`),b.length>0&&(i+=`- Data table \u2014 consider sortable columns and pagination
`),N.length>0&&(i+=`- Modal dialog \u2014 add overlay backdrop and focus trapping
`),u&&(i+=`- Multi-column footer with links
`),o==="detailed"||o==="forensic"){if(i+=`
### CSS Suggestions
`,f){let E=r.find(g=>g.type==="sidebar");i+=`- \`display: grid; grid-template-columns: ${Math.round(E.width)}px 1fr;\`
`}if(x.length>1){let E=Math.round(x[0].width);i+=`- \`display: grid; grid-template-columns: repeat(${x.length}, ${E}px); gap: 16px;\`
`}a&&(i+="- Navigation: `position: sticky; top: 0; z-index: 50;`\n")}return i}function K0(e,t="standard",n){let{sections:o}=e,r=[];for(let f of o){let u=f.originalRect,x=f.currentRect,S=Math.abs(u.x-x.x)>1||Math.abs(u.y-x.y)>1,b=Math.abs(u.width-x.width)>1||Math.abs(u.height-x.height)>1,N=!!f.note;if(!S&&!b&&!N){t==="forensic"&&r.push({section:f,posMoved:!1,sizeChanged:!1});continue}r.push({section:f,posMoved:S,sizeChanged:b})}if(r.length===0||t!=="forensic"&&r.every(f=>!f.posMoved&&!f.sizeChanged&&!f.section.note))return"";let i=`## Suggested Layout Changes

`,l=n?n.width:typeof window<"u"?window.innerWidth:0,s=n?n.height:typeof window<"u"?window.innerHeight:0,a=zg({width:l,height:s});t!=="compact"&&(i+=Ag(a)),t==="forensic"&&(i+=`> Detected at: \`${new Date(e.detectedAt).toISOString()}\`
`,i+=`> Total sections: ${o.length}

`);let c=f=>o.map(u=>({label:u.label,selector:u.selector,rect:f==="original"?u.originalRect:u.currentRect}));i+=`**Changes:**
`;for(let{section:f,posMoved:u,sizeChanged:x}of r){let S=f.originalRect,b=f.currentRect;if(!u&&!x){f.note?(i+=`- **${f.label}** \u2014 note only
`,i+=`  - Note: "${f.note}"
`):i+=`- ${f.label} \u2014 unchanged at (${Math.round(b.x)}, ${Math.round(b.y)}) ${Math.round(b.width)}\xD7${Math.round(b.height)}px
`;continue}if(t==="compact"){u&&x?i+=`- Suggested: move **${f.label}** to (${Math.round(b.x)}, ${Math.round(b.y)}) ${Math.round(b.width)}\xD7${Math.round(b.height)}px
`:u?i+=`- Suggested: move **${f.label}** to (${Math.round(b.x)}, ${Math.round(b.y)})
`:i+=`- Suggested: resize **${f.label}** to ${Math.round(b.width)}\xD7${Math.round(b.height)}px
`,f.note&&(i+=`  - Note: "${f.note}"
`);continue}if(u&&x?i+=`- Suggested: move and resize **${f.label}**
`:u?i+=`- Suggested: move **${f.label}**
`:i+=`- Suggested: resize **${f.label}** from ${Math.round(S.width)}\xD7${Math.round(S.height)}px to ${Math.round(b.width)}\xD7${Math.round(b.height)}px
`,f.note&&(i+=`  - Note: "${f.note}"
`),u){let E=U_(S,c("original")),g=U_(b,c("current")),v=x?{width:S.width,height:S.height}:void 0;i+=`  - Currently ${pv(E,{x:S.x,y:S.y},v)}
`;let h=x?{width:b.width,height:b.height}:void 0,C=`at (${Math.round(b.x)}, ${Math.round(b.y)})`,U=h?`, ${Math.round(h.width)}\xD7${Math.round(h.height)}px`:"",B=Bg(g,{includeLeftRight:t==="detailed"||t==="forensic"});if(B.length>0){i+=`  - Suggested position ${C}${U}: ${B[0]}
`;for(let F=1;F<B.length;F++)i+=`    ${B[F]}
`}else i+=`  - Suggested position ${C}${U}
`;let q=Og(b,a);q&&(i+=`  - CSS: ${q}
`)}let N=yv(f.selector);if(N&&(i+=`  - ${N}
`),i+=`  - Selector: \`${f.selector}\`
`,t==="detailed"||t==="forensic"){let E=f.className?`${f.tagName}.${f.className.split(" ")[0]}`:f.tagName;E!==f.selector&&(i+=`  - Element: \`${E}\`
`),f.role&&(i+=`  - Role: \`${f.role}\`
`),t==="forensic"&&f.textSnippet&&(i+=`  - Text: "${f.textSnippet}"
`)}t==="forensic"&&(i+=`  - Original rect: \`{ x: ${Math.round(S.x)}, y: ${Math.round(S.y)}, w: ${Math.round(S.width)}, h: ${Math.round(S.height)} }\`
`,i+=`  - Current rect: \`{ x: ${Math.round(b.x)}, y: ${Math.round(b.y)}, w: ${Math.round(b.width)}, h: ${Math.round(b.height)} }\`
`)}if(t!=="compact"){let f=r.filter(x=>x.posMoved).map(x=>({label:x.section.label,originalRect:x.section.originalRect,currentRect:x.section.currentRect})),u=mv(f);if(u.length>0){i+=`
### Layout Summary
`;for(let x of u)i+=`- ${x}
`}}if(t!=="compact"&&o.length>1){i+=`
### All Sections (current positions)
`;let f=[...o].sort((u,x)=>Math.abs(u.currentRect.y-x.currentRect.y)<20?u.currentRect.x-x.currentRect.x:u.currentRect.y-x.currentRect.y);for(let u of f){let x=u.currentRect,S=Math.abs(x.x-u.originalRect.x)>1||Math.abs(x.y-u.originalRect.y)>1||Math.abs(x.width-u.originalRect.width)>1||Math.abs(x.height-u.originalRect.height)>1;i+=`- ${u.label}: \`${Math.round(x.width)}\xD7${Math.round(x.height)}px\` at \`(${Math.round(x.x)}, ${Math.round(x.y)})\`${S?" \u2190 suggested":""}
`}}return i}var Y_="feedback-annotations-",Fg=7;function Z_(e){return`${Y_}${e}`}function kr(e){if(typeof window>"u")return[];try{let t=localStorage.getItem(Z_(e));if(!t)return[];let n=JSON.parse(t),o=Date.now()-Fg*24*60*60*1e3;return n.filter(r=>!r.timestamp||r.timestamp>o)}catch{return[]}}function Wg(e,t){if(!(typeof window>"u"))try{localStorage.setItem(Z_(e),JSON.stringify(t))}catch{}}function xv(){let e=new Map;if(typeof window>"u")return e;try{let t=Date.now()-Fg*24*60*60*1e3;for(let n=0;n<localStorage.length;n++){let o=localStorage.key(n);if(o?.startsWith(Y_)){let r=o.slice(Y_.length),i=localStorage.getItem(o);if(i){let s=JSON.parse(i).filter(a=>!a.timestamp||a.timestamp>t);s.length>0&&e.set(r,s)}}}}catch{}return e}function N_(e,t,n){let o=t.map(r=>({...r,_syncedTo:n}));Wg(e,o)}var ef="agentation-design-";function vv(e){if(typeof window>"u")return[];try{let t=localStorage.getItem(`${ef}${e}`);return t?JSON.parse(t):[]}catch{return[]}}function wv(e,t){if(!(typeof window>"u"))try{localStorage.setItem(`${ef}${e}`,JSON.stringify(t))}catch{}}function bv(e){if(!(typeof window>"u"))try{localStorage.removeItem(`${ef}${e}`)}catch{}}var tf="agentation-rearrange-";function kv(e){if(typeof window>"u")return null;try{let t=localStorage.getItem(`${tf}${e}`);return t?JSON.parse(t):null}catch{return null}}function Cv(e,t){if(!(typeof window>"u"))try{localStorage.setItem(`${tf}${e}`,JSON.stringify(t))}catch{}}function Sv(e){if(!(typeof window>"u"))try{localStorage.removeItem(`${tf}${e}`)}catch{}}var nf="agentation-wireframe-";function Mv(e){if(typeof window>"u")return null;try{let t=localStorage.getItem(`${nf}${e}`);return t?JSON.parse(t):null}catch{return null}}function J0(e,t){if(!(typeof window>"u"))try{localStorage.setItem(`${nf}${e}`,JSON.stringify(t))}catch{}}function yc(e){if(!(typeof window>"u"))try{localStorage.removeItem(`${nf}${e}`)}catch{}}var jg="agentation-session-";function of(e){return`${jg}${e}`}function Ev(e){if(typeof window>"u")return null;try{return localStorage.getItem(of(e))}catch{return null}}function I_(e,t){if(!(typeof window>"u"))try{localStorage.setItem(of(e),t)}catch{}}function Lv(e){if(!(typeof window>"u"))try{localStorage.removeItem(of(e))}catch{}}var Q_=`${jg}toolbar-hidden`;function Nv(){if(typeof window>"u")return!1;try{return sessionStorage.getItem(Q_)==="1"}catch{return!1}}function Iv(e){if(!(typeof window>"u"))try{e?sessionStorage.setItem(Q_,"1"):sessionStorage.removeItem(Q_)}catch{}}async function R_(e,t){let n=await fetch(`${e}/sessions`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({url:t})});if(!n.ok)throw new Error(`Failed to create session: ${n.status}`);return n.json()}async function Z0(e,t){let n=await fetch(`${e}/sessions/${t}`);if(!n.ok)throw new Error(`Failed to get session: ${n.status}`);return n.json()}async function eg(e,t,n){!n.elementPath&&(n.element==="body"||n.element==="html")&&(n={...n,elementPath:n.element});let o=await fetch(`${e}/sessions/${t}/annotations`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(n)});if(!o.ok)throw new Error(`Failed to sync annotation: ${o.status}`);return o.json()}async function $_(e,t,n){let o=await fetch(`${e}/annotations/${t}`,{method:"PATCH",headers:{"Content-Type":"application/json"},body:JSON.stringify(n)});if(!o.ok)throw new Error(`Failed to update annotation: ${o.status}`);return o.json()}async function xc(e,t){let n=await fetch(`${e}/annotations/${t}`,{method:"DELETE"});if(!n.ok)throw new Error(`Failed to delete annotation: ${n.status}`)}var lt={FunctionComponent:0,ClassComponent:1,IndeterminateComponent:2,HostRoot:3,HostPortal:4,HostComponent:5,HostText:6,Fragment:7,Mode:8,ContextConsumer:9,ContextProvider:10,ForwardRef:11,Profiler:12,SuspenseComponent:13,MemoComponent:14,SimpleMemoComponent:15,LazyComponent:16,IncompleteClassComponent:17,DehydratedFragment:18,SuspenseListComponent:19,ScopeComponent:21,OffscreenComponent:22,LegacyHiddenComponent:23,CacheComponent:24,TracingMarkerComponent:25,HostHoistable:26,HostSingleton:27,IncompleteFunctionComponent:28,Throw:29,ViewTransitionComponent:30,ActivityComponent:31},tg=new Set(["Component","PureComponent","Fragment","Suspense","Profiler","StrictMode","Routes","Route","Outlet","Root","ErrorBoundaryHandler","HotReload","Hot"]),ng=[/Boundary$/,/BoundaryHandler$/,/Provider$/,/Consumer$/,/^(Inner|Outer)/,/Router$/,/^Client(Page|Segment|Root)/,/^Segment(ViewNode|Node)$/,/^LayoutSegment/,/^Server(Root|Component|Render)/,/^RSC/,/Context$/,/^Hot(Reload)?$/,/^(Dev|React)(Overlay|Tools|Root)/,/Overlay$/,/Handler$/,/^With[A-Z]/,/Wrapper$/,/^Root$/],Rv=[/Page$/,/View$/,/Screen$/,/Section$/,/Card$/,/List$/,/Item$/,/Form$/,/Modal$/,/Dialog$/,/Button$/,/Nav$/,/Header$/,/Footer$/,/Layout$/,/Panel$/,/Tab$/,/Menu$/];function $v(e){let t=e?.mode??"filtered",n=tg;if(e?.skipExact){let o=e.skipExact instanceof Set?e.skipExact:new Set(e.skipExact);n=new Set([...tg,...o])}return{maxComponents:e?.maxComponents??6,maxDepth:e?.maxDepth??30,mode:t,skipExact:n,skipPatterns:e?.skipPatterns?[...ng,...e.skipPatterns]:ng,userPatterns:e?.userPatterns??Rv,filter:e?.filter}}function Tv(e){return e.replace(/([a-z])([A-Z])/g,"$1-$2").replace(/([A-Z])([A-Z][a-z])/g,"$1-$2").toLowerCase()}function Pv(e,t=10){let n=new Set,o=e,r=0;for(;o&&r<t;)o.className&&typeof o.className=="string"&&o.className.split(/\s+/).forEach(i=>{if(i.length>1){let l=i.replace(/[_][a-zA-Z0-9]{5,}.*$/,"").toLowerCase();l.length>1&&n.add(l)}}),o=o.parentElement,r++;return n}function Dv(e,t){let n=Tv(e);for(let o of t){if(o===n)return!0;let r=n.split("-").filter(l=>l.length>2),i=o.split("-").filter(l=>l.length>2);for(let l of r)for(let s of i)if(l===s||l.includes(s)||s.includes(l))return!0}return!1}function Bv(e,t,n,o){if(n.filter)return n.filter(e,t);switch(n.mode){case"all":return!0;case"filtered":return!(n.skipExact.has(e)||n.skipPatterns.some(r=>r.test(e)));case"smart":return n.skipExact.has(e)||n.skipPatterns.some(r=>r.test(e))?!1:!!(o&&Dv(e,o)||n.userPatterns.some(r=>r.test(e)));default:return!0}}var Xi=null,zv=new WeakMap;function T_(e){return Object.keys(e).some(t=>t.startsWith("__reactFiber$")||t.startsWith("__reactInternalInstance$")||t.startsWith("__reactProps$"))}function Ov(){if(Xi!==null)return Xi;if(typeof document>"u")return!1;if(document.body&&T_(document.body))return Xi=!0,!0;let e=["#root","#app","#__next","[data-reactroot]"];for(let t of e){let n=document.querySelector(t);if(n&&T_(n))return Xi=!0,!0}if(document.body){for(let t of document.body.children)if(T_(t))return Xi=!0,!0}return Xi=!1,!1}var us={map:zv};function Av(e){return Object.keys(e).find(n=>n.startsWith("__reactFiber$")||n.startsWith("__reactInternalInstance$"))||null}function Fv(e){let t=Av(e);return t?e[t]:null}function oi(e){return e?e.displayName?e.displayName:e.name?e.name:null:null}function Wv(e){let{tag:t,type:n,elementType:o}=e;if(t===lt.HostComponent||t===lt.HostText||t===lt.HostHoistable||t===lt.HostSingleton||t===lt.Fragment||t===lt.Mode||t===lt.Profiler||t===lt.DehydratedFragment||t===lt.HostRoot||t===lt.HostPortal||t===lt.ScopeComponent||t===lt.OffscreenComponent||t===lt.LegacyHiddenComponent||t===lt.CacheComponent||t===lt.TracingMarkerComponent||t===lt.Throw||t===lt.ViewTransitionComponent||t===lt.ActivityComponent)return null;if(t===lt.ForwardRef){let r=o;if(r?.render){let i=oi(r.render);if(i)return i}return r?.displayName?r.displayName:oi(n)}if(t===lt.MemoComponent||t===lt.SimpleMemoComponent){let r=o;if(r?.type){let i=oi(r.type);if(i)return i}return r?.displayName?r.displayName:oi(n)}if(t===lt.ContextProvider){let r=n;return r?._context?.displayName?`${r._context.displayName}.Provider`:null}if(t===lt.ContextConsumer){let r=n;return r?.displayName?`${r.displayName}.Consumer`:null}if(t===lt.LazyComponent){let r=o;return r?._status===1&&r._result?oi(r._result):null}return t===lt.SuspenseComponent||t===lt.SuspenseListComponent?null:t===lt.IncompleteClassComponent||t===lt.IncompleteFunctionComponent||t===lt.FunctionComponent||t===lt.ClassComponent||t===lt.IndeterminateComponent?oi(n):null}function jv(e){return e.length<=2||e.length<=3&&e===e.toLowerCase()}function Hv(e,t){let n=$v(t),o=n.mode==="all";if(o){let a=us.map.get(e);if(a!==void 0)return a}if(!Ov()){let a={path:null,components:[]};return o&&us.map.set(e,a),a}let r=n.mode==="smart"?Pv(e):void 0,i=[];try{let a=Fv(e),c=0;for(;a&&c<n.maxDepth&&i.length<n.maxComponents;){let f=Wv(a);f&&!jv(f)&&Bv(f,c,n,r)&&i.push(f),a=a.return,c++}}catch{let a={path:null,components:[]};return o&&us.map.set(e,a),a}if(i.length===0){let a={path:null,components:[]};return o&&us.map.set(e,a),a}let s={path:i.slice().reverse().map(a=>`<${a}>`).join(" "),components:i};return o&&us.map.set(e,s),s}var _s={FunctionComponent:0,ClassComponent:1,IndeterminateComponent:2,HostRoot:3,HostPortal:4,HostComponent:5,HostText:6,Fragment:7,Mode:8,ContextConsumer:9,ContextProvider:10,ForwardRef:11,Profiler:12,SuspenseComponent:13,MemoComponent:14,SimpleMemoComponent:15,LazyComponent:16};function Yv(e){if(!e||typeof e!="object")return null;let t=Object.keys(e),n=t.find(i=>i.startsWith("__reactFiber$"));if(n)return e[n]||null;let o=t.find(i=>i.startsWith("__reactInternalInstance$"));if(o)return e[o]||null;let r=t.find(i=>{if(!i.startsWith("__react"))return!1;let l=e[i];return l&&typeof l=="object"&&"_debugSource"in l});return r&&e[r]||null}function xs(e){if(!e.type||typeof e.type=="string")return null;if(typeof e.type=="object"||typeof e.type=="function"){let t=e.type;if(t.displayName)return t.displayName;if(t.name)return t.name}return null}function Qv(e,t=50){let n=e,o=0;for(;n&&o<t;){if(n._debugSource)return{source:n._debugSource,componentName:xs(n)};if(n._debugOwner?._debugSource)return{source:n._debugOwner._debugSource,componentName:xs(n._debugOwner)};n=n.return,o++}return null}function Vv(e){let t=e,n=0,o=50;for(;t&&n<o;){let r=t,i=["_debugSource","__source","_source","debugSource"];for(let l of i){let s=r[l];if(s&&typeof s=="object"&&"fileName"in s)return{source:s,componentName:xs(t)}}if(t.memoizedProps){let l=t.memoizedProps;if(l.__source&&typeof l.__source=="object"){let s=l.__source;if(s.fileName&&s.lineNumber)return{source:{fileName:s.fileName,lineNumber:s.lineNumber,columnNumber:s.columnNumber},componentName:xs(t)}}}t=t.return,n++}return null}var vc=new Map;function Xv(e){let t=e.tag,n=e.type,o=e.elementType;if(typeof n=="string"||n==null||typeof n=="function"&&n.prototype?.isReactComponent)return null;if((t===_s.FunctionComponent||t===_s.IndeterminateComponent)&&typeof n=="function")return n;if(t===_s.ForwardRef&&o){let r=o.render;if(typeof r=="function")return r}if((t===_s.MemoComponent||t===_s.SimpleMemoComponent)&&o){let r=o.type;if(typeof r=="function")return r}return typeof n=="function"?n:null}function Gv(){let e=Uv,t=e.__CLIENT_INTERNALS_DO_NOT_USE_OR_WARN_USERS_THEY_CANNOT_UPGRADE;if(t&&"H"in t)return{get:()=>t.H,set:o=>{t.H=o}};let n=e.__SECRET_INTERNALS_DO_NOT_USE_OR_YOU_WILL_BE_FIRED;if(n){let o=n.ReactCurrentDispatcher;if(o&&"current"in o)return{get:()=>o.current,set:r=>{o.current=r}}}return null}function qv(e,t){let n=e.split(`
`),o=[/source-location/,/\/dist\/index\./,/node_modules\//,/react-dom/,/react\.development/,/react\.production/,/chunk-[A-Z0-9]+/i,/\/_next\/static\/chunks\//,/\/\.vite\/deps\//,/\/_astro\//,/\/assets\/[^\s/]+[-.][\w-]{8,}\.m?js(?:[?:]|$)/,/react-stack-bottom-frame/,/react-reconciler/,/scheduler/,/<anonymous>/],r=/^\s*at\s+(?:.*?\s+\()?(.+?):(\d+):(\d+)\)?$/,i=/^[^@]*@(.+?):(\d+):(\d+)$/;for(let l of n){let s=l.trim();if(!s||o.some(c=>c.test(s)))continue;if(t){let c=t.replace(/^bound /,"").replace(/[.*+?^${}()|[\]\\]/g,"\\$&");if(!new RegExp(`(?:at (?:Object\\.)?|^)${c}(?: \\(|@| \\[)`).test(s))continue}let a=r.exec(s)||i.exec(s);if(a)return{fileName:a[1],line:parseInt(a[2],10),column:parseInt(a[3],10)}}return null}function Kv(e){let t=e;return t=t.replace(/[?#].*$/,""),t=t.replace(/^turbopack:\/\/\/\[project\]\//,""),t=t.replace(/^webpack-internal:\/\/\/\.\//,""),t=t.replace(/^webpack-internal:\/\/\//,""),t=t.replace(/^webpack:\/\/\/\.\//,""),t=t.replace(/^webpack:\/\/\//,""),t=t.replace(/^turbopack:\/\/\//,""),t=t.replace(/^https?:\/\/[^/]+\//,""),t=t.replace(/^file:\/\/\//,"/"),t=t.replace(/^\([^)]+\)\/\.\//,""),t=t.replace(/^\.\//,""),t}function Jv(e){let t=Xv(e);if(!t)return null;if(vc.has(t))return vc.get(t);let n=Gv();if(!n)return vc.set(t,null),null;let o=n.get(),r=null;try{let i=new Proxy({},{get(){throw new Error("probe")}});n.set(i);try{t({})}catch(l){if(l instanceof Error&&l.message==="probe"&&l.stack){let s=qv(l.stack,t.name);s&&(r={fileName:Kv(s.fileName),lineNumber:s.line,columnNumber:s.column,componentName:xs(e)||void 0})}}}finally{n.set(o)}return vc.set(t,r),r}function Zv(e,t=15){let n=e,o=0;for(;n&&o<t;){let r=Jv(n);if(r)return r;n=n.return,o++}return null}function V_(e){let t=Yv(e);if(!t)return{found:!1,reason:"no-fiber",isReactApp:!1,isProduction:!1};let n=Qv(t);if(n||(n=Vv(t)),n?.source)return{found:!0,source:{fileName:n.source.fileName,lineNumber:n.source.lineNumber,columnNumber:n.source.columnNumber,componentName:n.componentName||void 0},isReactApp:!0,isProduction:!1};let o=Zv(t);return o?{found:!0,source:o,isReactApp:!0,isProduction:!1}:{found:!1,reason:"no-debug-source",isReactApp:!0,isProduction:!1}}function ew(e,t="path"){let{fileName:n,lineNumber:o,columnNumber:r}=e,i=`${n}:${o}`;return r!==void 0&&(i+=`:${r}`),t==="vscode"?`vscode://file${n.startsWith("/")?"":"/"}${i}`:i}function tw(e,t=10){let n=e,o=0;for(;n&&o<t;){let r=V_(n);if(r.found)return r;n=n.parentElement,o++}return V_(e)}var fs=[{value:"compact",label:"Compact"},{value:"standard",label:"Standard"},{value:"detailed",label:"Detailed"},{value:"forensic",label:"Forensic"}];function Mc(e,t){let n=`## Page Feedback: ${e}
`,o=t?.replace(/[\r\n\t]+/g," ").trim();return o&&(n+=`**App:** ${o.replace(/[\\`*_\[\]<>]/g,"\\$&")}
`),n}function og(e,t,n="standard",o={}){if(e.length===0)return"";let r=typeof window<"u"?`${window.innerWidth}\xD7${window.innerHeight}`:"unknown",i=Mc(t,o.appName);return n==="forensic"?(i+=`
**Environment:**
`,i+=`- Viewport: ${r}
`,typeof window<"u"&&(i+=`- URL: ${window.location.href}
`,i+=`- User Agent: ${navigator.userAgent}
`,i+=`- Timestamp: ${new Date().toISOString()}
`,i+=`- Device Pixel Ratio: ${window.devicePixelRatio}
`),i+=`
---
`):n!=="compact"&&(i+=`**Viewport:** ${r}
`),i+=`
`,e.forEach((l,s)=>{n==="compact"?(i+=`${s+1}. **${l.element}**${l.sourceFile?` (${l.sourceFile})`:""}: ${l.comment}`,l.selectedText&&(i+=` (re: "${l.selectedText.slice(0,30)}${l.selectedText.length>30?"...":""}")`),i+=`
`):n==="forensic"?(i+=`### ${s+1}. ${l.element}
`,l.isMultiSelect&&l.fullPath&&(i+=`*Forensic data shown for first element of selection*
`),l.fullPath&&(i+=`**Full DOM Path:** ${l.fullPath}
`),l.cssClasses&&(i+=`**CSS Classes:** ${l.cssClasses}
`),l.boundingBox&&(i+=`**Position:** x:${Math.round(l.boundingBox.x)}, y:${Math.round(l.boundingBox.y)} (${Math.round(l.boundingBox.width)}\xD7${Math.round(l.boundingBox.height)}px)
`),i+=`**Annotation at:** ${l.x.toFixed(1)}% from left, ${Math.round(l.y)}px from top
`,l.selectedText&&(i+=`**Selected text:** "${l.selectedText}"
`),l.nearbyText&&!l.selectedText&&(i+=`**Context:** ${l.nearbyText.slice(0,100)}
`),l.computedStyles&&(i+=`**Computed Styles:** ${l.computedStyles}
`),l.accessibility&&(i+=`**Accessibility:** ${l.accessibility}
`),l.nearbyElements&&(i+=`**Nearby Elements:** ${l.nearbyElements}
`),l.sourceFile&&(i+=`**Source:** ${l.sourceFile}
`),l.reactComponents&&(i+=`**React:** ${l.reactComponents}
`),i+=`**Feedback:** ${l.comment}

`):(i+=`### ${s+1}. ${l.element}
`,i+=`**Location:** ${l.elementPath}
`,l.sourceFile&&(i+=`**Source:** ${l.sourceFile}
`),l.reactComponents&&(i+=`**React:** ${l.reactComponents}
`),n==="detailed"&&(l.cssClasses&&(i+=`**Classes:** ${l.cssClasses}
`),l.boundingBox&&(i+=`**Position:** ${Math.round(l.boundingBox.x)}px, ${Math.round(l.boundingBox.y)}px (${Math.round(l.boundingBox.width)}\xD7${Math.round(l.boundingBox.height)}px)
`)),l.selectedText&&(i+=`**Selected text:** "${l.selectedText}"
`),n==="detailed"&&l.nearbyText&&!l.selectedText&&(i+=`**Context:** ${l.nearbyText.slice(0,100)}
`),i+=`**Feedback:** ${l.comment}

`)}),i.trim()}function rg(e,t,n="markdown"){return n==="markdown"?t:[...new Set(e.map(o=>n==="source"?o.sourceFile:n==="classes"?o.cssClasses:o.attributes?.[n.attribute]).filter(o=>typeof o=="string"&&o.length>0))].join(`
`)}async function nw(e){if(typeof window>"u")return!1;try{if(navigator.clipboard?.writeText)return await navigator.clipboard.writeText(e),!0}catch{}return ow(e)}function ow(e){let t=document.createElement("textarea"),n=document.activeElement;for(;n?.shadowRoot?.activeElement;)n=n.shadowRoot.activeElement;let o=n instanceof HTMLInputElement||n instanceof HTMLTextAreaElement?n:null,r=o&&o.selectionStart!==null?{start:o.selectionStart,end:o.selectionEnd,direction:o.selectionDirection}:null,i=document.getSelection(),l=i?Array.from({length:i.rangeCount},(s,a)=>i.getRangeAt(a).cloneRange()):[];try{return t.value=e,t.setAttribute("readonly",""),t.style.cssText="position:fixed;left:-9999px;top:0;opacity:0;pointer-events:none;",document.body.appendChild(t),t.focus({preventScroll:!0}),t.select(),t.setSelectionRange(0,e.length),document.execCommand("copy")}catch{return!1}finally{if(t.remove(),n instanceof HTMLElement&&n.isConnected&&(n.focus({preventScroll:!0}),o&&r&&o.setSelectionRange(r.start,r.end,r.direction)),i){i.removeAllRanges();for(let s of l)i.addRange(s)}}}function ig(e){if(!e)return e;try{let t=new URL(e,"http://agentation.invalid");return t.pathname+t.search+t.hash}catch{return e}}function lg(e,t,n=[]){let o=new Map,r=new Map(n.map(s=>[s.id,s])),i=!1;async function l(s,a){if(i||a.running||a.timer)return;let c=a.desired,f=t.get(s);if(!c&&!f){o.delete(s),t.delete(s);return}let u=c?JSON.stringify(c):void 0;if(!(c&&f&&a.synced===u)){a.running=!0;try{if(!c)await e.remove(f),t.delete(s),a.synced=void 0;else if(f)await e.update(f,c),a.synced=u;else{t.set(s,"");let x=await e.create(c);t.set(s,x.id),a.synced=u}a.retries=0,a.running=!1,!i&&o.get(s)===a&&l(s,a)}catch(x){if(a.running=!1,!i&&o.get(s)===a&&(console.warn("[Agentation] Failed to sync layout feedback:",x),t.get(s)&&a.retries<3)){let S=500*2**a.retries++;a.timer=nt(()=>{a.timer=void 0,l(s,a)},S)}}}}return{replace(s){let a=new Map(s.map(c=>[c.id,c]));for(let[c,f]of a){let u=o.get(c);if(!u){u={running:!1,retries:0},o.set(c,u);let x=[...r.values()].find(S=>S.kind===f.kind&&ig(S.url)===ig(f.url)&&(f.kind==="placement"?S.timestamp===f.timestamp&&S.element===f.element:S.element===f.element));x&&(t.set(c,x.id),r.delete(x.id))}JSON.stringify(u.desired)!==JSON.stringify(f)&&(u.retries=0,u.timer&&clearTimeout(u.timer),u.timer=void 0),u.desired=f}for(let[c,f]of o)a.has(c)||(f.desired=void 0),l(c,f)},forget(s){let a=o.get(s);a?.timer&&clearTimeout(a.timer),o.delete(s),t.delete(s)},dispose(){i=!0;for(let s of o.values())s.timer&&clearTimeout(s.timer)}}}function rw(e,t,n,o){let r=!1,i,l,s=1e3,a,c=S=>{!r&&(S?.status==="resolved"||S?.status==="dismissed")&&o(S)},f=async()=>{if(r||a||!n())return;let S=new AbortController;a=S;let b=nt(()=>S.abort(),5e3);try{let N=await fetch(`${e}/sessions/${t}`,{signal:S.signal});if(!N.ok)return;let E=await N.json();!r&&!S.signal.aborted&&Array.isArray(E.annotations)&&E.annotations.forEach(c)}catch{}finally{clearTimeout(b),a===S&&(a=void 0)}},u=()=>{if(r)return;let S=new EventSource(`${e}/sessions/${t}/events`),b=()=>{s=1e3,f()},N=g=>{try{c(JSON.parse(g.data).payload)}catch{}},E=()=>{S.readyState!==EventSource.CLOSED||r||l!==void 0||(i?.(),l=nt(()=>{l=void 0,u()},s),s=Math.min(s*2,1e4))};S.addEventListener("open",b),S.addEventListener("annotation.updated",N),S.addEventListener("error",E),i=()=>{S.removeEventListener("open",b),S.removeEventListener("annotation.updated",N),S.removeEventListener("error",E),S.close()}};u();let x=gg(()=>{f()},1e4);return()=>{r=!0,i?.(),l!==void 0&&clearTimeout(l),clearInterval(x),a?.abort()}}var iw=`.styles-module__surface___7qnpJ {
  padding: 0;
  width: var(--preview-width, 200px);
  max-width: calc(100vw - 24px);
  overflow: auto;
  opacity: 0;
  visibility: hidden;
  pointer-events: none;
  border-radius: 12px;
  z-index: inherit;
  will-change: auto;
  transform: translateX(-50%);
  transition: left 200ms cubic-bezier(0.2, 0.8, 0.2, 1), top 200ms cubic-bezier(0.2, 0.8, 0.2, 1), transform 200ms cubic-bezier(0.2, 0.8, 0.2, 1), width 200ms cubic-bezier(0.2, 0.8, 0.2, 1), opacity 100ms ease-out, visibility 0s 200ms;
}
.styles-module__surface___7qnpJ[data-positioning] {
  transition: none;
}
.styles-module__surface___7qnpJ[data-direct-entry] *, .styles-module__surface___7qnpJ[data-direct-entry] *::before, .styles-module__surface___7qnpJ[data-direct-entry] *::after {
  transition: none !important;
}
.styles-module__surface___7qnpJ[data-state=preview], .styles-module__surface___7qnpJ[data-state=edit] {
  opacity: 1;
  visibility: visible;
  transition-delay: 0s;
}
.styles-module__surface___7qnpJ[data-state=edit], .styles-module__surface___7qnpJ[data-annotation-popup][data-state=hidden] {
  width: 280px;
  border-radius: 16px;
}
.styles-module__surface___7qnpJ[data-state=edit] {
  pointer-events: auto;
}
.styles-module__surface___7qnpJ[data-state=preview] {
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.3), 0 0 0 1px rgba(255, 255, 255, 0.08);
}
[data-agentation-theme=light] .styles-module__surface___7qnpJ[data-state=preview] {
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.12), 0 0 0 1px rgba(0, 0, 0, 0.06);
}

@media (prefers-reduced-motion: reduce) {
  .styles-module__surface___7qnpJ {
    transition: opacity 100ms ease-out, visibility 0s 100ms;
  }
}`,lw={surface:"styles-module__surface___7qnpJ"},sw=(0,Nn.forwardRef)(function({annotation:t,editing:n,exiting:o,restorePreview:r,editorProps:i,lightMode:l,scrollY:s,onExited:a},c){let f=(0,Nn.useRef)({annotation:t,editorProps:i}),u=t??f.current.annotation,x=t?i:f.current.editorProps,S=(0,Nn.useRef)(null),b=(0,Nn.useRef)(null),N=(0,Nn.useRef)(),E=(0,Nn.useRef)(s),g=n&&!o?"edit":t&&(!n||r)?"preview":"hidden",v=g==="preview"||!n;return(0,Nn.useLayoutEffect)(()=>{t&&(f.current={annotation:t,editorProps:i})},[t,i]),(0,Nn.useLayoutEffect)(()=>{let h=S.current;if(!h||!u)return;let C=g==="edit"&&(N.current!==u.id||h.dataset.state==="hidden"&&getComputedStyle(h).opacity==="0");C&&(h.dataset.directEntry="true");let U=()=>{let q=u.x/100*window.innerWidth,F=u.isFixed?u.y:u.y-s,K=g==="preview"||!n,se=parseFloat(h.style.getPropertyValue("--preview-width"))||200,Z=Math.min(se,window.innerWidth-24),fe=Math.max(12,Math.min(window.innerWidth-Z-12,q-Z/2)),oe=K?Z:Math.min(280,window.innerWidth-24),ae=Math.min(K?12:20,(window.innerWidth-oe)/2),pe=F>window.innerHeight-(K?101:290);h.style.left=`${Math.max(ae,Math.min(window.innerWidth-oe-ae,fe))}px`,h.style.right="auto",h.style.top=`${Math.max(12,Math.min(window.innerHeight-12,F+(pe?-21:21)))}px`,h.style.bottom="auto",h.style.transform=pe?"translateY(-100%)":"translateY(0)",h.style.maxHeight=`${Math.max(100,pe?F-33:window.innerHeight-F-33)}px`},V=N.current!==u.id||E.current!==s||h.dataset.state==="hidden";V&&(h.dataset.positioning="true"),U(),(V||C)&&h.getBoundingClientRect(),delete h.dataset.positioning,delete h.dataset.directEntry,h.dataset.state=g,h.inert=g!=="edit",N.current=u.id,E.current=s;let B=()=>{h.dataset.positioning="true",U(),h.getBoundingClientRect(),delete h.dataset.positioning};return window.addEventListener("resize",B),()=>window.removeEventListener("resize",B)},[u?.id,u?.x,u?.y,u?.isFixed,g,n,s,u?.comment]),(0,Nn.useLayoutEffect)(()=>{n&&!o&&b.current?.focus()},[n,o,u?.id]),q_(S,o,a),(0,Nn.useImperativeHandle)(c,()=>({shake(){S.current?.animate?.([{translate:"0px"},{translate:"-3px"},{translate:"3px"},{translate:"-2px"},{translate:"2px"},{translate:"0px"}],{duration:250}),b.current?.focus()}}),[]),!u||!x?null:sg("div",{ref:S,className:`${Be.popup} ${lw.surface} ${l?Be.light:""}`,"data-feedback-toolbar":!0,"data-annotation-card":!0,"data-annotation-popup":n?"":void 0,"data-state":"hidden","aria-hidden":g==="hidden",onClick:h=>h.stopPropagation(),onKeyDownCapture:h=>{h.key!=="Escape"||h.nativeEvent.isComposing||!n||(h.preventDefault(),h.stopPropagation(),x.onCancel())},children:sg(Lg,{ref:b,...x,variant:"card",preview:v,resetOnPreview:!n,disabled:!n||o},u.id)})}),aw=`@keyframes styles-module__markerIn___x4G8D {
  0% {
    opacity: 0;
    transform: translate(-50%, -50%) scale(0.3);
  }
  100% {
    opacity: 1;
    transform: translate(-50%, -50%) scale(1);
  }
}
@keyframes styles-module__markerOut___6VhQN {
  0% {
    opacity: 1;
    transform: translate(-50%, -50%) scale(1);
  }
  100% {
    opacity: 0;
    transform: translate(-50%, -50%) scale(0.3);
  }
}
@keyframes styles-module__renumberRoll___akV9B {
  0% {
    transform: translateX(-40%);
    opacity: 0;
  }
  100% {
    transform: translateX(0);
    opacity: 1;
  }
}
.styles-module__marker___9CKF7 {
  padding: 0;
  border: 0;
  box-sizing: border-box;
  font-family: inherit;
  line-height: 1;
  text-align: center;
  appearance: none;
  position: absolute;
  width: 22px;
  height: 22px;
  background: var(--agentation-color-blue);
  color: white;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 0.6875rem;
  font-weight: 600;
  transform: translate(-50%, -50%) scale(1);
  opacity: 1;
  cursor: pointer;
  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.2), inset 0 0 0 1px rgba(0, 0, 0, 0.04);
  -webkit-user-select: none;
  user-select: none;
  will-change: transform, opacity;
  contain: layout style;
  z-index: 1;
}
.styles-module__marker___9CKF7 > * {
  pointer-events: none;
}
.styles-module__marker___9CKF7:focus-visible {
  outline: 2px solid var(--agentation-color-accent);
  outline-offset: 3px;
}
.styles-module__marker___9CKF7:hover, .styles-module__marker___9CKF7:focus-visible, .styles-module__marker___9CKF7.styles-module__previewVisible___imMag {
  z-index: 2;
}
.styles-module__marker___9CKF7:not(.styles-module__enter___8kI3q):not(.styles-module__exit___KBdR3):not(.styles-module__clearing___8rM7K):not(.styles-module__confirm___BtMvq) {
  transition: background-color 0.15s ease, transform 0.1s ease, z-index 0s 0.1s;
}
.styles-module__marker___9CKF7:not(.styles-module__enter___8kI3q):not(.styles-module__exit___KBdR3):not(.styles-module__clearing___8rM7K):not(.styles-module__confirm___BtMvq):hover, .styles-module__marker___9CKF7:not(.styles-module__enter___8kI3q):not(.styles-module__exit___KBdR3):not(.styles-module__clearing___8rM7K):not(.styles-module__confirm___BtMvq):focus-visible, .styles-module__marker___9CKF7:not(.styles-module__enter___8kI3q):not(.styles-module__exit___KBdR3):not(.styles-module__clearing___8rM7K):not(.styles-module__confirm___BtMvq).styles-module__previewVisible___imMag {
  transition-delay: 0s;
}
.styles-module__marker___9CKF7.styles-module__enter___8kI3q {
  animation: styles-module__markerIn___x4G8D 0.25s cubic-bezier(0.22, 1, 0.36, 1) both;
}
.styles-module__marker___9CKF7.styles-module__confirm___BtMvq {
  animation: styles-module__markerConfirm___RT4Sk 220ms ease-out both;
}
.styles-module__marker___9CKF7.styles-module__exit___KBdR3 {
  animation: styles-module__markerOut___6VhQN 0.2s ease-out both;
  pointer-events: none;
}
.styles-module__marker___9CKF7.styles-module__clearing___8rM7K {
  animation: styles-module__markerOut___6VhQN 0.15s ease-out both;
  pointer-events: none;
}
.styles-module__marker___9CKF7:not(.styles-module__enter___8kI3q):not(.styles-module__exit___KBdR3):not(.styles-module__clearing___8rM7K):not(.styles-module__confirm___BtMvq):hover {
  transform: translate(-50%, -50%) scale(1.1);
}
.styles-module__marker___9CKF7.styles-module__pending___BiY-U {
  background-color: var(--agentation-color-blue);
  cursor: default;
}
.styles-module__marker___9CKF7.styles-module__pending___BiY-U.styles-module__exit___KBdR3 {
  animation-duration: 150ms;
}
.styles-module__marker___9CKF7.styles-module__multiSelect___CPfTC {
  background-color: var(--agentation-color-green);
  width: 26px;
  height: 26px;
  border-radius: 6px;
  font-size: 0.75rem;
}
.styles-module__marker___9CKF7.styles-module__multiSelect___CPfTC.styles-module__pending___BiY-U {
  background-color: var(--agentation-color-green);
}
.styles-module__marker___9CKF7.styles-module__hovered___-mg2N {
  background-color: var(--agentation-color-red);
}

.styles-module__renumber___16lvD {
  display: block;
  animation: styles-module__renumberRoll___akV9B 0.2s ease-out;
}

@keyframes styles-module__markerConfirm___RT4Sk {
  0% {
    transform: translate(-50%, -50%) scale(1);
  }
  25% {
    transform: translate(-50%, -50%) scale(0.94);
  }
  65% {
    transform: translate(-50%, -50%) scale(1.06);
  }
  100% {
    transform: translate(-50%, -50%) scale(1);
  }
}
.styles-module__number___1JFu9 {
  display: block;
}

.styles-module__numberGlyph___qchdk {
  display: block;
  opacity: 1;
  transform: translateY(0);
  filter: blur(0);
  transition: opacity 140ms ease-out, transform 180ms cubic-bezier(0.22, 1, 0.36, 1), filter 140ms ease-out;
}

.styles-module__actionGlyph___AFRt0 {
  position: absolute;
  inset: 0;
  display: grid;
  place-items: center;
  opacity: 0;
  transform: translateY(2px) scale(0.8) rotate(-12deg);
  filter: blur(1px);
  transition: opacity 120ms ease-out, transform 160ms cubic-bezier(0.22, 1, 0.36, 1), filter 120ms ease-out;
}

.styles-module__actionVisible___Kb--l .styles-module__numberGlyph___qchdk {
  opacity: 0;
  transform: translateY(-2px) scale(0.8);
  filter: blur(1px);
}
.styles-module__actionVisible___Kb--l .styles-module__actionGlyph___AFRt0 {
  opacity: 1;
  transform: translateY(0) scale(1) rotate(0);
  filter: blur(0);
}

.styles-module__plus___xslMP {
  position: absolute;
  inset: 0;
  display: grid;
  place-items: center;
  opacity: 0;
  transform: rotate(-90deg) scale(0.6);
  filter: blur(1px);
  transition: opacity 100ms ease-out, transform 140ms ease-out, filter 100ms ease-out;
  pointer-events: none;
}

.styles-module__pending___BiY-U .styles-module__numberGlyph___qchdk {
  opacity: 0;
  transform: translateY(5px);
  filter: blur(2px);
}
.styles-module__pending___BiY-U .styles-module__plus___xslMP {
  opacity: 1;
  transform: rotate(0) scale(1);
  filter: blur(0);
}

@media (prefers-reduced-motion: reduce) {
  .styles-module__marker___9CKF7.styles-module__enter___8kI3q, .styles-module__marker___9CKF7.styles-module__exit___KBdR3, .styles-module__marker___9CKF7.styles-module__clearing___8rM7K, .styles-module__marker___9CKF7.styles-module__confirm___BtMvq {
    animation-duration: 1ms;
    animation-delay: 0ms !important;
  }
  .styles-module__numberGlyph___qchdk, .styles-module__actionGlyph___AFRt0, .styles-module__plus___xslMP {
    transition: opacity 100ms ease-out;
    transform: none;
    filter: none;
  }
  .styles-module__pending___BiY-U .styles-module__numberGlyph___qchdk, .styles-module__pending___BiY-U .styles-module__plus___xslMP,
  .styles-module__actionVisible___Kb--l .styles-module__numberGlyph___qchdk, .styles-module__actionVisible___Kb--l .styles-module__actionGlyph___AFRt0 {
    transform: none;
    filter: none;
  }
}`,tn={marker:"styles-module__marker___9CKF7",previewVisible:"styles-module__previewVisible___imMag",enter:"styles-module__enter___8kI3q",exit:"styles-module__exit___KBdR3",clearing:"styles-module__clearing___8rM7K",confirm:"styles-module__confirm___BtMvq",markerIn:"styles-module__markerIn___x4G8D",markerConfirm:"styles-module__markerConfirm___RT4Sk",markerOut:"styles-module__markerOut___6VhQN",pending:"styles-module__pending___BiY-U",multiSelect:"styles-module__multiSelect___CPfTC",hovered:"styles-module__hovered___-mg2N",renumber:"styles-module__renumber___16lvD",renumberRoll:"styles-module__renumberRoll___akV9B",number:"styles-module__number___1JFu9",numberGlyph:"styles-module__numberGlyph___qchdk",actionGlyph:"styles-module__actionGlyph___AFRt0",actionVisible:"styles-module__actionVisible___Kb--l",plus:"styles-module__plus___xslMP"},ag=(0,Jn.memo)(function({annotation:t,pending:n=!1,globalIndex:o,layerIndex:r,layerSize:i,isExiting:l,isClearing:s,isAnimated:a,isNew:c,isHovered:f,isRemoving:u,onRemoveComplete:x,isEditingAny:S,renumberFrom:b,markerClickBehavior:N,onHoverEnter:E,onEnterComplete:g,onHoverLeave:v,onClick:h,onContextMenu:C}){let[U,V]=(0,Jn.useState)(a),B=(0,Jn.useRef)(n),[q,F]=(0,Jn.useState)(!1),K=B.current&&!n&&U&&!q;(0,Jn.useLayoutEffect)(()=>{l&&V(!1)},[l]);let se=(0,Jn.useRef)(null),Z=(0,Jn.useRef)({action:!1,delete:!1}),fe=f&&!S,oe=fe&&N==="delete";(0,Jn.useLayoutEffect)(()=>{u||(Z.current={action:fe,delete:oe})},[u,fe,oe]);let ae=u?Z.current.action:fe,pe=u?Z.current.delete:oe;q_(se,u,()=>x(t.id));let kt=t.isMultiSelect,Oe=kt?"var(--agentation-color-green)":"var(--agentation-color-accent)",qe=s?tn.clearing:l||u?tn.exit:K?tn.confirm:!a&&!U?tn.enter:"",Ae=s?`${Math.min(r*20,120)}ms`:u||n||K?"0ms":l?`${(i-1-r)*20}ms`:`${c?0:r*20}ms`;return cw("button",{ref:se,type:"button","aria-label":n?"Pending annotation":`${N==="delete"?"Delete":"Edit"} annotation ${o+1}: ${t.element}`,disabled:n||l||u||s,tabIndex:n||S?-1:0,className:`${tn.marker} ${n?tn.pending:""} ${kt?tn.multiSelect:""} ${qe} ${!n&&ae?tn.actionVisible:""} ${pe?tn.hovered:""} ${f&&!S&&!u?tn.previewVisible:""}`,"data-annotation-marker":n?void 0:"","data-annotation-pending":n?"":void 0,style:{left:`${t.x}%`,top:t.y,backgroundColor:pe?void 0:Oe,animationDelay:Ae},onAnimationEnd:_t=>{_t.target===_t.currentTarget&&(qe===tn.enter||qe===tn.confirm)&&(V(!0),n||F(!0),n||g(t.id))},onMouseOver:()=>{n||E(t)},onMouseOut:_t=>{let qt=_t.relatedTarget;(!(qt instanceof Node)||!_t.currentTarget.contains(qt))&&v(t.id)},onFocus:_t=>{!n&&_t.currentTarget.matches(":focus-visible")&&E(t)},onBlur:()=>v(t.id),onClick:_t=>{_t.stopPropagation(),!n&&!l&&!u&&h(t,_t.currentTarget)},onContextMenu:C?_t=>{N==="delete"&&(_t.preventDefault(),_t.stopPropagation(),!n&&!l&&!u&&C(t,_t.currentTarget))}:void 0,children:[ri("span",{className:`${tn.number} ${b!==null&&o>=b?tn.renumber:""}`,"aria-hidden":"true",children:ri("span",{className:tn.numberGlyph,children:o+1})},o),ri("span",{className:tn.actionGlyph,"aria-hidden":"true",children:N==="delete"?ri(b2,{size:kt?18:16}):ri(S2,{size:16})}),B.current&&ri("span",{className:tn.plus,"aria-hidden":"true",children:ri(h2,{size:12})})]})}),dw=`.styles-module__switchContainer___Ka-AB {
  display: flex;
  align-items: center;
  position: relative;
  padding: 2px;
  width: 24px;
  height: 16px;
  border-radius: 8px;
  background-color: #cdcdcd;
  transition: background-color 0.15s, opacity 0.15s;
}
[data-agentation-theme=dark] .styles-module__switchContainer___Ka-AB {
  background-color: #484848;
}
.styles-module__switchContainer___Ka-AB:has(.styles-module__switchInput___kYDSD:checked) {
  background-color: var(--agentation-color-blue);
}
.styles-module__switchContainer___Ka-AB:has(.styles-module__switchInput___kYDSD:disabled) {
  opacity: 0.3;
}

.styles-module__switchInput___kYDSD {
  position: absolute;
  z-index: 1;
  inset: 0;
  border-radius: inherit;
  opacity: 0;
  cursor: pointer;
}
.styles-module__switchInput___kYDSD:disabled {
  cursor: not-allowed;
}

.styles-module__switchThumb___4sCPH {
  border-radius: 50%;
  width: 12px;
  height: 12px;
  background-color: #fff;
  transition: transform 0.15s;
}
.styles-module__switchContainer___Ka-AB[data-checked] .styles-module__switchThumb___4sCPH {
  transform: translateX(8px);
}`,P_={switchContainer:"styles-module__switchContainer___Ka-AB",switchInput:"styles-module__switchInput___kYDSD",switchThumb:"styles-module__switchThumb___4sCPH"},D_=({className:e="",checked:t,onChange:n,...o})=>uw("div",{className:`${P_.switchContainer} ${e}`,"data-checked":t?"":void 0,children:[cg("input",{className:P_.switchInput,checked:t,onChange:n,type:"checkbox",...o}),cg("div",{className:P_.switchThumb})]}),_w=`.styles-module__checkboxContainer___joqZk {
  display: flex;
  justify-content: center;
  align-items: center;
  position: relative;
  border: 1px solid rgba(26, 26, 26, 0.2);
  border-radius: 4px;
  width: 14px;
  height: 14px;
  background-color: #fff;
  transition: background-color 0.2s ease;
}
[data-agentation-theme=dark] .styles-module__checkboxContainer___joqZk {
  border-color: rgba(255, 255, 255, 0.2);
  background-color: #252525;
}
.styles-module__checkboxContainer___joqZk:has(.styles-module__checkboxInput___ECzzO:checked) {
  background-color: #1a1a1a;
}
[data-agentation-theme=dark] .styles-module__checkboxContainer___joqZk:has(.styles-module__checkboxInput___ECzzO:checked) {
  background-color: #fff;
}

.styles-module__checkboxInput___ECzzO {
  position: absolute;
  z-index: 1;
  inset: -1px;
  border-radius: inherit;
  opacity: 0;
  cursor: pointer;
}

.styles-module__checkboxCheck___fUXpr {
  color: #fafafa;
}
[data-agentation-theme=dark] .styles-module__checkboxCheck___fUXpr {
  color: #1a1a1a;
}

.styles-module__checkboxCheckPath___cDyh8 {
  stroke-dasharray: 9.29px;
  stroke-dashoffset: 9.29px;
  color: #fafafa;
  transition: stroke-dashoffset 0.1s ease;
}
[data-agentation-theme=dark] .styles-module__checkboxCheckPath___cDyh8 {
  color: #1a1a1a;
}
.styles-module__checkboxContainer___joqZk[data-checked] .styles-module__checkboxCheckPath___cDyh8 {
  transition-duration: 0.2s;
  stroke-dashoffset: 0;
}`,wc={checkboxContainer:"styles-module__checkboxContainer___joqZk",checkboxInput:"styles-module__checkboxInput___ECzzO",checkboxCheck:"styles-module__checkboxCheck___fUXpr",checkboxCheckPath:"styles-module__checkboxCheckPath___cDyh8"},hw=({className:e="",checked:t,onChange:n,...o})=>fw("div",{className:`${wc.checkboxContainer} ${e}`,"data-checked":t?"":void 0,children:[B_("input",{className:wc.checkboxInput,type:"checkbox",checked:t,onChange:n,...o}),B_("svg",{className:wc.checkboxCheck,width:"14",height:"14",viewBox:"0 0 14 14",fill:"none",children:B_("path",{className:wc.checkboxCheckPath,d:"M3.94 7L6.13 9.19L10.5 4.81",stroke:"currentColor",strokeWidth:"1.5",strokeLinecap:"round",strokeLinejoin:"round"})})]}),pw=`.styles-module__container___w8eAF {
  display: flex;
  align-items: center;
  height: 24px;
}

.styles-module__label___J5mxE {
  padding-inline: 8px 2px;
  line-height: 20px;
  font-size: 13px;
  letter-spacing: -0.15px;
  color: rgba(26, 26, 26, 0.5);
  -webkit-user-select: none;
  user-select: none;
  cursor: pointer;
}
[data-agentation-theme=dark] .styles-module__label___J5mxE {
  color: rgba(255, 255, 255, 0.5);
}`,dg={container:"styles-module__container___w8eAF",label:"styles-module__label___J5mxE"},ug=({className:e="",label:t,tooltip:n,checked:o,onChange:r,...i})=>{let l=(0,Hg.useId)();return mw("div",{className:`${dg.container} ${e}`,...i,children:[z_(hw,{id:l,onChange:r,checked:o}),z_("label",{className:dg.label,htmlFor:l,children:t}),n&&z_(ii,{content:n})]})},gw=`@keyframes styles-module__cycleTextIn___VBNTi {
  0% {
    opacity: 0;
    transform: translateY(-6px);
  }
  100% {
    opacity: 1;
    transform: translateY(0);
  }
}
@keyframes styles-module__scaleIn___QpQ8E {
  from {
    opacity: 0;
    transform: scale(0.85);
  }
  to {
    opacity: 1;
    transform: scale(1);
  }
}
@keyframes styles-module__mcpPulse___5Q3Jj {
  0% {
    box-shadow: 0 0 0 0 color-mix(in srgb, var(--agentation-color-green) 50%, transparent);
  }
  70% {
    box-shadow: 0 0 0 6px color-mix(in srgb, var(--agentation-color-green) 0%, transparent);
  }
  100% {
    box-shadow: 0 0 0 0 color-mix(in srgb, var(--agentation-color-green) 0%, transparent);
  }
}
@keyframes styles-module__mcpPulseError___VHxhx {
  0% {
    box-shadow: 0 0 0 0 color-mix(in srgb, var(--agentation-color-red) 50%, transparent);
  }
  70% {
    box-shadow: 0 0 0 6px color-mix(in srgb, var(--agentation-color-red) 0%, transparent);
  }
  100% {
    box-shadow: 0 0 0 0 color-mix(in srgb, var(--agentation-color-red) 0%, transparent);
  }
}
@keyframes styles-module__themeIconIn___qUWMV {
  0% {
    opacity: 0;
    transform: scale(0.8) rotate(-30deg);
  }
  100% {
    opacity: 1;
    transform: scale(1) rotate(0deg);
  }
}
.styles-module__settingsPanel___qNkn- :where(button, a, input, select, textarea):focus-visible {
  outline: 2px solid var(--agentation-color-accent);
  outline-offset: 2px;
}
.styles-module__settingsPanel___qNkn- {
  position: absolute;
  right: 5px;
  bottom: calc(100% + 0.5rem);
  z-index: 1;
  overflow: hidden;
  background: #1c1c1c;
  border-radius: 16px;
  padding: 12px 0;
  width: 253px;
  max-width: calc(100vw - 20px);
  cursor: default;
  opacity: 1;
  box-shadow: 0 1px 8px rgba(0, 0, 0, 0.25), 0 0 0 1px rgba(0, 0, 0, 0.04);
  transition: background-color 0.25s ease, box-shadow 0.25s ease;
}
.styles-module__settingsPanel___qNkn-::before, .styles-module__settingsPanel___qNkn-::after {
  content: "";
  position: absolute;
  top: 0;
  bottom: 0;
  width: 16px;
  z-index: 2;
  pointer-events: none;
}
.styles-module__settingsPanel___qNkn-::before {
  left: 0;
  background: linear-gradient(to right, #1c1c1c 0%, transparent 100%);
}
.styles-module__settingsPanel___qNkn-::after {
  right: 0;
  background: linear-gradient(to left, #1c1c1c 0%, transparent 100%);
}
.styles-module__settingsPanel___qNkn- .styles-module__settingsHeader___Fn1DP,
.styles-module__settingsPanel___qNkn- .styles-module__settingsBrand___OoKlM,
.styles-module__settingsPanel___qNkn- .styles-module__settingsVersion___rXmL9,
.styles-module__settingsPanel___qNkn- .styles-module__settingsSection___n5V-4,
.styles-module__settingsPanel___qNkn- .styles-module__settingsLabel___VCVOQ,
.styles-module__settingsPanel___qNkn- .styles-module__cycleButton___XMBx3,
.styles-module__settingsPanel___qNkn- .styles-module__cycleDot___zgSXY,
.styles-module__settingsPanel___qNkn- .styles-module__dropdownButton___mKHe8,
.styles-module__settingsPanel___qNkn- .styles-module__sliderLabel___6K5v1,
.styles-module__settingsPanel___qNkn- .styles-module__slider___v5z-c,
.styles-module__settingsPanel___qNkn- .styles-module__themeToggle___3imlT {
  transition: background-color 0.25s ease, color 0.25s ease, border-color 0.25s ease;
}
.styles-module__settingsPanel___qNkn- {
  opacity: 0;
  transform: translateY(var(--panel-offset-y, 4px)) scale(0.98);
  transform-origin: var(--panel-origin, bottom right);
  filter: blur(2px);
  pointer-events: none;
  visibility: hidden;
  transition: opacity 120ms cubic-bezier(0.25, 0.46, 0.45, 0.94), transform 120ms cubic-bezier(0.25, 0.46, 0.45, 0.94), filter 120ms cubic-bezier(0.25, 0.46, 0.45, 0.94);
}
.styles-module__settingsPanel___qNkn-[data-panel-present=true] {
  visibility: visible;
}
.styles-module__settingsPanel___qNkn-[data-panel-open=true] {
  opacity: 1;
  transform: translateY(0) scale(1);
  filter: blur(0);
  pointer-events: auto;
  transition-duration: 160ms;
}
@media (prefers-reduced-motion: reduce) {
  .styles-module__settingsPanel___qNkn- {
    transition: none;
    transform: none;
    filter: none;
  }
}
.styles-module__settingsPanel___qNkn-.styles-module__below___Vpv-k {
  --panel-offset-y: -4px;
  --panel-origin: top right;
}
[data-agentation-theme=dark] .styles-module__settingsPanel___qNkn- {
  background: #1a1a1a;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.3), 0 0 0 1px rgba(255, 255, 255, 0.08);
}
[data-agentation-theme=dark] .styles-module__settingsPanel___qNkn- .styles-module__settingsLabel___VCVOQ {
  color: rgba(255, 255, 255, 0.6);
}
[data-agentation-theme=dark] .styles-module__settingsPanel___qNkn- .styles-module__settingsOption___JoyH- {
  color: rgba(255, 255, 255, 0.85);
}
[data-agentation-theme=dark] .styles-module__settingsPanel___qNkn- .styles-module__settingsOption___JoyH-:hover {
  background: rgba(255, 255, 255, 0.1);
}
[data-agentation-theme=dark] .styles-module__settingsPanel___qNkn- .styles-module__settingsOption___JoyH-.styles-module__selected___k1-Vq {
  background: rgba(255, 255, 255, 0.15);
  color: #fff;
}

.styles-module__settingsPanelContainer___5it-H {
  overflow: visible;
  position: relative;
  display: flex;
  padding: 0 16px;
}

.styles-module__settingsPage___BMn-3 {
  min-width: 100%;
  flex-basis: 0;
  flex-shrink: 0;
  transition: transform 0.2s ease, opacity 0.2s ease;
  transition-delay: 0s;
  opacity: 1;
}

.styles-module__settingsPage___BMn-3.styles-module__slideLeft___qUvW4 {
  transform: translateX(-24px);
  opacity: 0;
  pointer-events: none;
}

.styles-module__automationsPage___N7By0 {
  position: absolute;
  top: 0;
  left: 24px;
  width: 100%;
  height: 100%;
  padding: 0 16px 4px;
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  transition: transform 0.2s ease, opacity 0.2s ease;
  opacity: 0;
  pointer-events: none;
}

.styles-module__automationsPage___N7By0.styles-module__slideIn___uXDSu {
  transform: translateX(-24px);
  opacity: 1;
  pointer-events: auto;
}

.styles-module__settingsHeader___Fn1DP {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 24px;
}

.styles-module__settingsBrand___OoKlM {
  display: flex;
  align-items: center;
  font-size: 0.8125rem;
  font-weight: 500;
  letter-spacing: -0.0094em;
  color: #bbb;
  text-decoration: none;
}

.styles-module__settingsVersion___rXmL9 {
  font-size: 11px;
  font-weight: 400;
  color: rgba(255, 255, 255, 0.4);
  margin-left: 6px;
  letter-spacing: -0.0094em;
}

.styles-module__themeToggle___3imlT {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  margin-left: auto;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: rgba(255, 255, 255, 0.4);
  transition: background-color 0.15s ease, color 0.15s ease;
  cursor: pointer;
}
.styles-module__themeToggle___3imlT:hover {
  background: rgba(255, 255, 255, 0.1);
  color: rgba(255, 255, 255, 0.8);
}
[data-agentation-theme=light] .styles-module__themeToggle___3imlT {
  color: rgba(0, 0, 0, 0.4);
}
[data-agentation-theme=light] .styles-module__themeToggle___3imlT:hover {
  background: rgba(0, 0, 0, 0.06);
  color: rgba(0, 0, 0, 0.7);
}

.styles-module__themeIconWrapper___pyaYa {
  display: flex;
  align-items: center;
  justify-content: center;
  position: relative;
  width: 20px;
  height: 20px;
}

.styles-module__themeIcon___w7lAm {
  display: flex;
  align-items: center;
  justify-content: center;
  animation: styles-module__themeIconIn___qUWMV 0.35s cubic-bezier(0.34, 1.56, 0.64, 1) forwards;
}

.styles-module__settingsSectionGrow___eZTRw {
  flex: 1;
  display: flex;
  flex-direction: column;
}

.styles-module__settingsRow___y-tDE {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 24px;
}
.styles-module__settingsRow___y-tDE.styles-module__settingsRowMarginTop___uLpGb {
  margin-top: 8px;
}

.styles-module__settingsRowDisabled___ydl3Q .styles-module__settingsLabel___VCVOQ {
  color: rgba(255, 255, 255, 0.2);
}
[data-agentation-theme=light] .styles-module__settingsRowDisabled___ydl3Q .styles-module__settingsLabel___VCVOQ {
  color: rgba(0, 0, 0, 0.2);
}

.styles-module__settingsLabel___VCVOQ {
  display: flex;
  align-items: center;
  column-gap: 2px;
  line-height: 20px;
  font-size: 13px;
  font-weight: 400;
  letter-spacing: -0.15px;
  color: rgba(255, 255, 255, 0.5);
}
[data-agentation-theme=light] .styles-module__settingsLabel___VCVOQ {
  color: rgba(0, 0, 0, 0.5);
}

.styles-module__cycleButton___XMBx3 {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0;
  border: none;
  background: transparent;
  font-size: 0.8125rem;
  font-weight: 500;
  color: #fff;
  cursor: pointer;
  letter-spacing: -0.0094em;
}
[data-agentation-theme=light] .styles-module__cycleButton___XMBx3 {
  color: rgba(0, 0, 0, 0.85);
}
.styles-module__cycleButton___XMBx3:disabled {
  opacity: 0.35;
  cursor: not-allowed;
}

.styles-module__cycleButtonText___mbbnD {
  display: inline-block;
  animation: styles-module__cycleTextIn___VBNTi 0.2s ease-out;
}

.styles-module__cycleDots___ehp6i {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.styles-module__cycleDot___zgSXY {
  width: 3px;
  height: 3px;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.3);
  transform: scale(0.667);
  transition: background-color 0.25s ease-out, transform 0.25s ease-out;
}
.styles-module__cycleDot___zgSXY.styles-module__active___dpAhM {
  background: #fff;
  transform: scale(1);
}
[data-agentation-theme=light] .styles-module__cycleDot___zgSXY {
  background: rgba(0, 0, 0, 0.2);
}
[data-agentation-theme=light] .styles-module__cycleDot___zgSXY.styles-module__active___dpAhM {
  background: rgba(0, 0, 0, 0.7);
}

.styles-module__colorOptions___pbxZx {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 6px;
  height: 26px;
}

.styles-module__colorOption___Co955 {
  padding: 0;
  position: relative;
  border-radius: 50%;
  width: 20px;
  height: 20px;
  background-color: #fff;
  cursor: pointer;
}
[data-agentation-theme=dark] .styles-module__colorOption___Co955 {
  background-color: #1a1a1a;
}
.styles-module__colorOption___Co955::before, .styles-module__colorOption___Co955::after {
  content: "";
  position: absolute;
  inset: 0;
  border-radius: 50%;
  background-color: var(--swatch);
  transition: opacity 0.2s, transform 0.2s;
}
@supports (color: color(display-p3 0 0 0)) {
  .styles-module__colorOption___Co955::before, .styles-module__colorOption___Co955::after {
    --color: var(--swatch-p3);
  }
}
.styles-module__colorOption___Co955::after {
  z-index: -1;
  transform: scale(1.2);
  opacity: 0;
}
.styles-module__colorOption___Co955.styles-module__selected___k1-Vq::before {
  transform: scale(0.8);
}
.styles-module__colorOption___Co955.styles-module__selected___k1-Vq::after {
  opacity: 1;
}

.styles-module__settingsNavLink___uYIwM {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  height: 24px;
  padding: 0;
  border: none;
  background: transparent;
  font-family: inherit;
  line-height: 20px;
  font-size: 13px;
  font-weight: 400;
  color: rgba(255, 255, 255, 0.5);
  transition: color 0.15s ease;
  cursor: pointer;
}
.styles-module__settingsNavLink___uYIwM:hover {
  color: rgba(255, 255, 255, 0.9);
}
.styles-module__settingsNavLink___uYIwM svg {
  color: rgba(255, 255, 255, 0.4);
  transition: color 0.15s ease;
}
.styles-module__settingsNavLink___uYIwM:hover svg {
  color: #fff;
}
[data-agentation-theme=light] .styles-module__settingsNavLink___uYIwM {
  color: rgba(0, 0, 0, 0.5);
}
[data-agentation-theme=light] .styles-module__settingsNavLink___uYIwM:hover {
  color: rgba(0, 0, 0, 0.8);
}
[data-agentation-theme=light] .styles-module__settingsNavLink___uYIwM svg {
  color: rgba(0, 0, 0, 0.25);
}
[data-agentation-theme=light] .styles-module__settingsNavLink___uYIwM:hover svg {
  color: rgba(0, 0, 0, 0.8);
}

.styles-module__settingsNavLinkRight___XBUzC {
  display: flex;
  align-items: center;
  gap: 6px;
}

.styles-module__settingsBackButton___fflll {
  display: flex;
  align-items: center;
  gap: 4px;
  height: 24px;
  border: none;
  background: transparent;
  font-family: inherit;
  line-height: 20px;
  font-size: 13px;
  font-weight: 500;
  letter-spacing: -0.15px;
  color: #fff;
  cursor: pointer;
  transition: transform 0.12s cubic-bezier(0.32, 0.72, 0, 1);
}
.styles-module__settingsBackButton___fflll svg {
  opacity: 0.4;
  flex-shrink: 0;
  transition: opacity 0.15s ease, transform 0.18s cubic-bezier(0.32, 0.72, 0, 1);
}
.styles-module__settingsBackButton___fflll:hover svg {
  opacity: 1;
}
[data-agentation-theme=light] .styles-module__settingsBackButton___fflll {
  color: rgba(0, 0, 0, 0.85);
  border-bottom-color: rgba(0, 0, 0, 0.08);
}

.styles-module__automationHeader___Avra9 {
  display: flex;
  align-items: center;
  gap: 0.125rem;
  font-size: 0.8125rem;
  font-weight: 400;
  color: #fff;
}
[data-agentation-theme=light] .styles-module__automationHeader___Avra9 {
  color: rgba(0, 0, 0, 0.85);
}

.styles-module__automationDescription___vFTmJ {
  font-size: 0.6875rem;
  font-weight: 300;
  color: rgba(255, 255, 255, 0.5);
  margin-top: 2px;
  line-height: 14px;
}
[data-agentation-theme=light] .styles-module__automationDescription___vFTmJ {
  color: rgba(0, 0, 0, 0.5);
}

.styles-module__learnMoreLink___cG7OI {
  color: rgba(255, 255, 255, 0.8);
  text-decoration-line: underline;
  text-decoration-style: dotted;
  text-decoration-color: rgba(255, 255, 255, 0.2);
  text-underline-offset: 2px;
  transition: color 0.15s ease;
}
.styles-module__learnMoreLink___cG7OI:hover {
  color: #fff;
}
[data-agentation-theme=light] .styles-module__learnMoreLink___cG7OI {
  color: rgba(0, 0, 0, 0.6);
  text-decoration-color: rgba(0, 0, 0, 0.2);
}
[data-agentation-theme=light] .styles-module__learnMoreLink___cG7OI:hover {
  color: rgba(0, 0, 0, 0.85);
}

.styles-module__autoSendContainer___VpkXk {
  display: flex;
  align-items: center;
}

.styles-module__autoSendLabel___ngNdC {
  padding-inline-end: 8px;
  font-size: 11px;
  font-weight: 400;
  color: rgba(255, 255, 255, 0.4);
  transition: color 0.15s, opacity 0.15s;
  cursor: pointer;
}
.styles-module__autoSendLabel___ngNdC.styles-module__active___dpAhM {
  color: #66b8ff;
  color: color(display-p3 0.4 0.72 1);
}
[data-agentation-theme=light] .styles-module__autoSendLabel___ngNdC {
  color: rgba(0, 0, 0, 0.4);
}
[data-agentation-theme=light] .styles-module__autoSendLabel___ngNdC.styles-module__active___dpAhM {
  color: var(--agentation-color-blue);
}
.styles-module__autoSendLabel___ngNdC.styles-module__disabled___9AZYS {
  opacity: 0.3;
  cursor: not-allowed;
}

.styles-module__mcpStatusDot___8AMxP {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}
.styles-module__mcpStatusDot___8AMxP.styles-module__connecting___QEO1r {
  background-color: var(--agentation-color-yellow);
  animation: styles-module__mcpPulse___5Q3Jj 1.5s infinite;
}
.styles-module__mcpStatusDot___8AMxP.styles-module__connected___WyFkx {
  background-color: var(--agentation-color-green);
  animation: styles-module__mcpPulse___5Q3Jj 2.5s ease-in-out infinite;
}
.styles-module__mcpStatusDot___8AMxP.styles-module__disconnected___mvmvQ {
  background-color: var(--agentation-color-red);
  animation: styles-module__mcpPulseError___VHxhx 2s infinite;
}

.styles-module__mcpNavIndicator___auBHI {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}
.styles-module__mcpNavIndicator___auBHI.styles-module__connected___WyFkx {
  background-color: var(--agentation-color-green);
  animation: styles-module__mcpPulse___5Q3Jj 2.5s ease-in-out infinite;
}
.styles-module__mcpNavIndicator___auBHI.styles-module__connecting___QEO1r {
  background-color: var(--agentation-color-yellow);
  animation: styles-module__mcpPulse___5Q3Jj 1.5s ease-in-out infinite;
}

.styles-module__webhookUrlInput___WDDDC {
  display: block;
  width: 100%;
  flex: 1;
  min-height: 60px;
  box-sizing: border-box;
  margin-top: 11px;
  padding: 8px 10px;
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 6px;
  background: rgba(255, 255, 255, 0.03);
  font-family: inherit;
  font-size: 0.75rem;
  font-weight: 400;
  color: #fff;
  outline: none;
  resize: none;
  user-select: text;
  transition: border-color 0.15s ease, background-color 0.15s ease, box-shadow 0.15s ease;
}
.styles-module__webhookUrlInput___WDDDC::placeholder {
  color: rgba(255, 255, 255, 0.3);
}
.styles-module__webhookUrlInput___WDDDC:focus {
  border-color: rgba(255, 255, 255, 0.3);
  background: rgba(255, 255, 255, 0.08);
}
[data-agentation-theme=light] .styles-module__webhookUrlInput___WDDDC {
  border-color: rgba(0, 0, 0, 0.1);
  background: rgba(0, 0, 0, 0.03);
  color: rgba(0, 0, 0, 0.85);
}
[data-agentation-theme=light] .styles-module__webhookUrlInput___WDDDC::placeholder {
  color: rgba(0, 0, 0, 0.3);
}
[data-agentation-theme=light] .styles-module__webhookUrlInput___WDDDC:focus {
  border-color: rgba(0, 0, 0, 0.25);
  background: rgba(0, 0, 0, 0.05);
}

[data-agentation-theme=light] .styles-module__settingsPanel___qNkn- {
  background: #fff;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08), 0 4px 16px rgba(0, 0, 0, 0.06), 0 0 0 1px rgba(0, 0, 0, 0.04);
}
[data-agentation-theme=light] .styles-module__settingsPanel___qNkn-::before {
  background: linear-gradient(to right, #fff 0%, transparent 100%);
}
[data-agentation-theme=light] .styles-module__settingsPanel___qNkn-::after {
  background: linear-gradient(to left, #fff 0%, transparent 100%);
}
[data-agentation-theme=light] .styles-module__settingsPanel___qNkn- .styles-module__settingsHeader___Fn1DP {
  border-bottom-color: rgba(0, 0, 0, 0.08);
}
[data-agentation-theme=light] .styles-module__settingsPanel___qNkn- .styles-module__settingsBrand___OoKlM {
  color: #333;
}
[data-agentation-theme=light] .styles-module__settingsPanel___qNkn- .styles-module__settingsVersion___rXmL9 {
  color: rgba(0, 0, 0, 0.4);
}
[data-agentation-theme=light] .styles-module__settingsPanel___qNkn- .styles-module__settingsSection___n5V-4 {
  border-top-color: rgba(0, 0, 0, 0.08);
}
[data-agentation-theme=light] .styles-module__settingsPanel___qNkn- .styles-module__settingsLabel___VCVOQ {
  color: rgba(0, 0, 0, 0.5);
}
[data-agentation-theme=light] .styles-module__settingsPanel___qNkn- .styles-module__cycleButton___XMBx3 {
  color: rgba(0, 0, 0, 0.85);
}
[data-agentation-theme=light] .styles-module__settingsPanel___qNkn- .styles-module__cycleDot___zgSXY {
  background: rgba(0, 0, 0, 0.2);
}
[data-agentation-theme=light] .styles-module__settingsPanel___qNkn- .styles-module__cycleDot___zgSXY.styles-module__active___dpAhM {
  background: rgba(0, 0, 0, 0.7);
}
[data-agentation-theme=light] .styles-module__settingsPanel___qNkn- .styles-module__dropdownButton___mKHe8 {
  color: rgba(0, 0, 0, 0.85);
}
[data-agentation-theme=light] .styles-module__settingsPanel___qNkn- .styles-module__dropdownButton___mKHe8:hover {
  background: rgba(0, 0, 0, 0.05);
}

.styles-module__checkboxField___ZrSqv:not(:first-child) {
  margin-top: 8px;
}

.styles-module__divider___h6Yux {
  margin-block: 8px;
  width: 100%;
  height: 1px;
  background-color: rgba(26, 26, 26, 0.07);
}
[data-agentation-theme=dark] .styles-module__divider___h6Yux {
  background-color: rgba(255, 255, 255, 0.07);
}`,le={settingsPanel:"styles-module__settingsPanel___qNkn-",settingsHeader:"styles-module__settingsHeader___Fn1DP",settingsBrand:"styles-module__settingsBrand___OoKlM",settingsVersion:"styles-module__settingsVersion___rXmL9",settingsSection:"styles-module__settingsSection___n5V-4",settingsLabel:"styles-module__settingsLabel___VCVOQ",cycleButton:"styles-module__cycleButton___XMBx3",cycleDot:"styles-module__cycleDot___zgSXY",dropdownButton:"styles-module__dropdownButton___mKHe8",sliderLabel:"styles-module__sliderLabel___6K5v1",slider:"styles-module__slider___v5z-c",themeToggle:"styles-module__themeToggle___3imlT",below:"styles-module__below___Vpv-k",settingsOption:"styles-module__settingsOption___JoyH-",selected:"styles-module__selected___k1-Vq",settingsPanelContainer:"styles-module__settingsPanelContainer___5it-H",settingsPage:"styles-module__settingsPage___BMn-3",slideLeft:"styles-module__slideLeft___qUvW4",automationsPage:"styles-module__automationsPage___N7By0",slideIn:"styles-module__slideIn___uXDSu",themeIconWrapper:"styles-module__themeIconWrapper___pyaYa",themeIcon:"styles-module__themeIcon___w7lAm",themeIconIn:"styles-module__themeIconIn___qUWMV",settingsSectionGrow:"styles-module__settingsSectionGrow___eZTRw",settingsRow:"styles-module__settingsRow___y-tDE",settingsRowMarginTop:"styles-module__settingsRowMarginTop___uLpGb",settingsRowDisabled:"styles-module__settingsRowDisabled___ydl3Q",cycleButtonText:"styles-module__cycleButtonText___mbbnD",cycleTextIn:"styles-module__cycleTextIn___VBNTi",cycleDots:"styles-module__cycleDots___ehp6i",active:"styles-module__active___dpAhM",colorOptions:"styles-module__colorOptions___pbxZx",colorOption:"styles-module__colorOption___Co955",settingsNavLink:"styles-module__settingsNavLink___uYIwM",settingsNavLinkRight:"styles-module__settingsNavLinkRight___XBUzC",settingsBackButton:"styles-module__settingsBackButton___fflll",automationHeader:"styles-module__automationHeader___Avra9",automationDescription:"styles-module__automationDescription___vFTmJ",learnMoreLink:"styles-module__learnMoreLink___cG7OI",autoSendContainer:"styles-module__autoSendContainer___VpkXk",autoSendLabel:"styles-module__autoSendLabel___ngNdC",disabled:"styles-module__disabled___9AZYS",mcpStatusDot:"styles-module__mcpStatusDot___8AMxP",connecting:"styles-module__connecting___QEO1r",mcpPulse:"styles-module__mcpPulse___5Q3Jj",connected:"styles-module__connected___WyFkx",disconnected:"styles-module__disconnected___mvmvQ",mcpPulseError:"styles-module__mcpPulseError___VHxhx",mcpNavIndicator:"styles-module__mcpNavIndicator___auBHI",webhookUrlInput:"styles-module__webhookUrlInput___WDDDC",checkboxField:"styles-module__checkboxField___ZrSqv",divider:"styles-module__divider___h6Yux",scaleIn:"styles-module__scaleIn___QpQ8E"},yw=(0,Sr.memo)(function({settings:t,onSettingsChange:n,isDarkMode:o,onToggleTheme:r,isDevMode:i,connectionStatus:l,endpoint:s,onExited:a,isOpen:c,toolbarNearBottom:f,settingsPage:u,onSettingsPageChange:x,onHideToolbar:S}){let{ref:b}=Rg(c,{keepMounted:!0,onExited:a}),N=(0,Sr.useRef)(null),E=(0,Sr.useRef)(null),g=(0,Sr.useRef)(!1);(0,Sr.useLayoutEffect)(()=>{!c||!g.current||(g.current=!1,(u==="automations"?E:N).current?.focus())},[c,u]);let v=o?"Switch to light mode":"Switch to dark mode";return De("div",{className:`${le.settingsPanel} ${f?le.below:""}`,style:f?{bottom:"auto",top:"calc(100% + 0.5rem)"}:void 0,"data-agentation-settings-panel":!0,ref:h=>{b.current=h,h?.toggleAttribute("inert",!c)},role:"group","aria-label":"Feedback settings","aria-hidden":!c,children:bt("div",{className:le.settingsPanelContainer,children:[bt("div",{className:`${le.settingsPage} ${u==="automations"?le.slideLeft:""}`,ref:h=>{h?.toggleAttribute("inert",u!=="main")},"aria-hidden":u!=="main",children:[bt("div",{className:le.settingsHeader,children:[De("a",{className:le.settingsBrand,href:"https://agentation.com",target:"_blank",rel:"noopener noreferrer","aria-label":"Agentation",children:"Agentation"}),bt("p",{className:le.settingsVersion,children:["v","3.1.2"]}),De("button",{className:le.themeToggle,onClick:r,title:v,"aria-label":v,children:De("span",{className:le.themeIconWrapper,children:De("span",{className:le.themeIcon,children:o?De(k2,{size:20}):De(C2,{size:20})},o?"sun":"moon")})})]}),De("div",{className:le.divider}),bt("div",{className:le.settingsSection,children:[bt("div",{className:le.settingsRow,children:[bt("div",{className:le.settingsLabel,children:["Output Detail",De(ii,{content:"Controls how much detail is included in the copied output"})]}),bt("button",{className:le.cycleButton,onClick:()=>{let C=(fs.findIndex(U=>U.value===t.outputDetail)+1)%fs.length;n({outputDetail:fs[C].value})},children:[De("span",{className:le.cycleButtonText,children:fs.find(h=>h.value===t.outputDetail)?.label},t.outputDetail),De("span",{className:le.cycleDots,children:fs.map(h=>De("span",{className:`${le.cycleDot} ${t.outputDetail===h.value?le.active:""}`},h.value))})]})]}),bt("div",{className:`${le.settingsRow} ${le.settingsRowMarginTop} ${i?"":le.settingsRowDisabled}`,children:[bt("div",{className:le.settingsLabel,children:["React Components",De(ii,{content:i?"Include React component names in annotations":"Disabled \u2014 production builds minify component names, making detection unreliable. Use in development mode."})]}),De(D_,{"aria-label":"React Components",checked:i&&t.reactEnabled,onChange:h=>n({reactEnabled:h.target.checked}),disabled:!i})]}),bt("div",{className:`${le.settingsRow} ${le.settingsRowMarginTop}`,children:[bt("div",{className:le.settingsLabel,children:["Hide Until Restart",De(ii,{content:"Hides the toolbar until you open a new tab"})]}),De(D_,{"aria-label":"Hide Until Restart",checked:!1,onChange:h=>{h.target.checked&&S()}})]})]}),De("div",{className:le.divider}),bt("div",{className:le.settingsSection,children:[De("div",{className:`${le.settingsLabel} ${le.settingsLabelMarker}`,children:"Marker Color"}),De("div",{className:le.colorOptions,children:gs.map(h=>De("button",{className:`${le.colorOption} ${t.annotationColorId===h.id?le.selected:""}`,style:{"--swatch":h.srgb,"--swatch-p3":h.p3},onClick:()=>n({annotationColorId:h.id}),title:h.label,"aria-label":h.label,type:"button"},h.id))})]}),De("div",{className:le.divider}),bt("div",{className:le.settingsSection,children:[De(ug,{className:"checkbox-field",label:"Clear on copy/send",checked:t.autoClearAfterCopy,onChange:h=>n({autoClearAfterCopy:h.target.checked}),tooltip:"Automatically clear annotations after copying"}),De(ug,{className:le.checkboxField,label:"Block page interactions",checked:t.blockInteractions,onChange:h=>n({blockInteractions:h.target.checked})})]}),De("div",{className:le.divider}),bt("button",{className:le.settingsNavLink,ref:N,onClick:h=>{g.current=h.detail===0,h.currentTarget.blur(),x("automations")},children:[De("span",{children:"Manage MCP & Webhooks"}),bt("span",{className:le.settingsNavLinkRight,children:[s&&l!=="disconnected"&&De("span",{className:`${le.mcpNavIndicator} ${le[l]}`}),De("svg",{width:"16",height:"16",viewBox:"0 0 16 16",fill:"none",xmlns:"http://www.w3.org/2000/svg",children:De("path",{d:"M7.5 12.5L12 8L7.5 3.5",stroke:"currentColor",strokeWidth:"1.5",strokeLinecap:"round",strokeLinejoin:"round"})})]})]})]}),bt("div",{className:`${le.settingsPage} ${le.automationsPage} ${u==="automations"?le.slideIn:""}`,ref:h=>{h?.toggleAttribute("inert",u!=="automations")},"aria-hidden":u!=="automations",children:[bt("button",{className:le.settingsBackButton,ref:E,"aria-label":"Back to settings",onClick:h=>{g.current=h.detail===0,h.currentTarget.blur(),x("main")},children:[De(M2,{size:16}),De("span",{children:"Manage MCP & Webhooks"})]}),De("div",{className:le.divider}),bt("div",{className:le.settingsSection,children:[bt("div",{className:le.settingsRow,children:[bt("span",{className:le.automationHeader,children:["MCP Connection",De(ii,{content:"Connect via Model Context Protocol to let AI agents like Claude Code receive annotations in real-time."})]}),s&&De("div",{className:`${le.mcpStatusDot} ${le[l]}`,title:l==="connected"?"Connected":l==="connecting"?"Connecting...":"Disconnected"})]}),bt("p",{className:le.automationDescription,style:{paddingBottom:6},children:["MCP connection allows agents to receive and act on annotations."," ",De("a",{href:"https://agentation.com/mcp",target:"_blank",rel:"noopener noreferrer",className:le.learnMoreLink,children:"Learn more"})]})]}),De("div",{className:le.divider}),bt("div",{className:`${le.settingsSection} ${le.settingsSectionGrow}`,children:[bt("div",{className:le.settingsRow,children:[bt("span",{className:le.automationHeader,children:["Webhooks",De(ii,{content:"Send annotation data to any URL endpoint when annotations change. Useful for custom integrations."})]}),bt("div",{className:le.autoSendContainer,children:[De("label",{htmlFor:"agentation-auto-send",className:`${le.autoSendLabel} ${t.webhooksEnabled?le.active:""} ${t.webhookUrl?"":le.disabled}`,children:"Auto-Send"}),De(D_,{id:"agentation-auto-send",checked:t.webhooksEnabled,onChange:h=>n({webhooksEnabled:h.target.checked}),disabled:!t.webhookUrl})]})]}),De("p",{className:le.automationDescription,children:"The webhook URL will receive live annotation changes and annotation data."}),De("textarea",{className:le.webhookUrlInput,placeholder:"Webhook URL","aria-label":"Webhook URL",value:t.webhookUrl,onKeyDown:h=>h.stopPropagation(),onChange:h=>n({webhookUrl:h.target.value})})]})]})]})})});function vw({x:e,y:t,elementName:n,reactComponents:o}){let r=(0,Tc.useRef)(null);return(0,Tc.useLayoutEffect)(()=>{let i=r.current;if(!i)return;let l=()=>{let s=i.offsetWidth,a=i.offsetHeight;i.style.left=`${Math.max(8,Math.min(e,window.innerWidth-s-8))}px`;let c=t-a-8;i.style.top=`${Math.max(8,Math.min(c,window.innerHeight-a-8))}px`};return l(),window.addEventListener("resize",l),()=>window.removeEventListener("resize",l)},[e,t,n,o]),xw("div",{ref:r,className:`${O.hoverTooltip} ${O.enter}`,children:[o&&_g("div",{className:O.hoverReactPath,children:o}),_g("div",{className:O.hoverElementName,children:n})]})}var ww=`@charset "UTF-8";
/* Reset box-model and set borders */
/* ============================================ */
*,
::before,
::after {
  border-width: 0;
  border-style: solid;
  box-sizing: border-box;
}

/* Document */
/* ============================================ */
/**
 * 1. Correct line height in all browsers.
 * 2. Prevent adjustments of font size after orientation changes in iOS.
 * 3. Remove gray overlay on links for iOS.
 * 4. Render kerning consistently in all browsers.
 * 5. Correct font smoothing for macOS.
 */
:host {
  /* Inherited properties cross the shadow boundary, so a host page's
     text-transform, letter-spacing or font would otherwise restyle the UI. */
  font: 400 16px/1.5 system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
  font-variant: normal;
  color: initial;
  letter-spacing: normal;
  word-spacing: normal;
  text-transform: none;
  text-align: start;
  text-indent: 0;
  text-shadow: none;
  white-space: normal;
  direction: ltr;
  writing-mode: horizontal-tb;
  hyphens: manual;
  word-break: normal;
  overflow-wrap: normal;
  tab-size: 8;
  list-style: none;
  quotes: initial;
  caret-color: auto;
  user-select: auto;
  -webkit-text-fill-color: initial;
  -webkit-text-stroke: 0;
  text-rendering: auto;
  -webkit-text-size-adjust: 100%; /* 2 */
  -webkit-tap-highlight-color: transparent; /* 3 */
  font-feature-settings: "kern"; /* 4 */
  -webkit-font-feature-settings: "kern"; /* 5 */
  -moz-font-feature-settings: "kern"; /* 5 */
  -webkit-font-smoothing: antialiased; /* 5 */
  -moz-osx-font-smoothing: grayscale; /* 5 */
}

/* Vertical rhythm */
/* ============================================ */
p,
table,
blockquote,
address,
pre,
iframe,
form,
figure,
dl {
  margin: 0;
}

/* Headings */
/* ============================================ */
h1,
h2,
h3,
h4,
h5,
h6 {
  margin: 0;
  font-size: inherit;
  font-weight: inherit;
}

/* Lists (enumeration) */
/* ============================================ */
ul,
ol,
menu {
  list-style: none;
  margin: 0;
  padding: 0;
}

/* Lists (definition) */
/* ============================================ */
dd {
  margin-left: 0;
}

/* Grouping content */
/* ============================================ */
/**
 * 1. Add the correct box sizing in Firefox.
 * 2. Show the overflow in Edge and IE.
 */
hr {
  clear: both;
  margin: 0;
  border-top-width: 1px;
  height: 0; /* 1 */
  box-sizing: content-box; /* 1 */
  overflow: visible; /* 2 */
  color: inherit;
}

/**
 * 1. Correct the inheritance and scaling of font size in all browsers.
 * 2. Correct the odd \`em\` font sizing in all browsers.
 * 3. Wrap lines by default instead of overflow.
 */
pre {
  font-family: inherit; /* 1 */
  font-size: inherit; /* 2 */
  white-space: pre-line; /* 3 */
}

address {
  font-style: inherit;
}

/* Text-level semantics */
/* ============================================ */
/**
 * Remove the gray background on active links in IE 10.
 */
a {
  background-color: transparent;
  text-decoration: none;
  color: inherit;
}

/**
 * 1. Remove the bottom border in Chrome 57-
 * 2. Add the correct text decoration in Chrome, Edge, IE, Opera, and Safari.
 */
abbr[title] {
  border-bottom: none; /* 1 */
  text-decoration: none; /* 2 */
}

/**
 * Add the correct font weight in Chrome, Edge, and Safari.
 */
b,
strong {
  font-weight: bolder;
}

/**
 * 1. Correct the inheritance and scaling of font size in all browsers.
 * 2. Correct the odd \`em\` font sizing in all browsers.
 */
code,
kbd,
samp {
  font-family: "Menlo", "Monaco", "Consolas", "Courier New", monospace; /* 1 */
  font-size: inherit; /* 2 */
}

/**
 * Add the correct font size in all browsers.
 */
small {
  font-size: 80%;
}

/**
 * Prevent \`sub\` and \`sup\` elements from affecting the line height in all browsers.
 */
sub,
sup {
  position: relative;
  vertical-align: baseline;
  line-height: 0;
  font-size: 75%;
}

sub {
  bottom: -0.25em;
}

sup {
  top: -0.5em;
}

/* Replaced content */
/* ============================================ */
/**
 * Prevent vertical alignment issues.
 */
svg,
img,
embed,
object,
iframe {
  vertical-align: bottom;
}

/*
 * 1. Remove image default bottom space.
 * 2. Prevent image from overflowing the container.
 */
img {
  display: block;
  max-width: 100%;
}

/**
 * Prevent alignment issues on Safari.
 */
@supports (background: -webkit-named-image(i)) {
  svg {
    will-change: transform;
  }
}
/* Forms */
/* ============================================ */
/**
 * Reset form fields to make them styleable.
 * 1. Make form elements stylable across systems iOS especially.
 * 2. Inherit text-transform from parent.
 */
button,
input,
optgroup,
select,
textarea {
  -webkit-appearance: none; /* 1 */
  appearance: none;
  border-radius: 0;
  margin: 0;
  padding: 0;
  background: transparent;
  vertical-align: middle;
  text-align: inherit;
  text-transform: inherit; /* 2 */
  font: inherit;
  color: inherit;
}

/**
 * Correct cursors for clickable elements.
 */
button,
[type=button],
[type=reset],
[type=submit] {
  cursor: pointer;
}

button:disabled,
[type=button]:disabled,
[type=reset]:disabled,
[type=submit]:disabled {
  cursor: default;
}

/**
 * Clickable labels and selects.
 */
select,
label {
  cursor: pointer;
}

/**
 * Improve outlines for Firefox and unify style with input elements & buttons.
 */
:-moz-focusring {
  outline: auto;
}

select:disabled {
  opacity: inherit;
}

/**
 * 1. Remove padding.
 */
option {
  padding: 0; /* 1 */
}

/**
 * Reset to invisible
 */
fieldset {
  margin: 0;
  padding: 0;
  min-width: 0;
}

legend {
  display: contents;
  padding: 0;
}

/**
 * Add the correct vertical alignment in Chrome, Firefox, and Opera.
 */
progress {
  vertical-align: baseline;
}

/**
 * Remove the default vertical scrollbar in IE 10+.
 */
textarea {
  overflow: auto;
}

/**
 * Remove increment and decrement buttons in Chrome.
 */
[type=number]::-webkit-inner-spin-button,
[type=number]::-webkit-outer-spin-button {
  -webkit-appearance: none;
}

/**
 * Correct the outline style in Safari.
 */
[type=search] {
  outline-offset: -2px;
}

/**
 * Remove the inner padding in Chrome and Safari on macOS.
 */
[type=search]::-webkit-search-decoration {
  -webkit-appearance: none;
}

/*
 * Remove the \u2018X\u2019 from Chrome and Safari.
 */
[type=search]::-webkit-search-decoration,
[type=search]::-webkit-search-cancel-button,
[type=search]::-webkit-search-results-button,
[type=search]::-webkit-search-results-decoration {
  display: none;
}

/**
 * 1. Hide file input completely.
 * 2. Remove selected file text.
 * 3. Set cursor to pointer for all browsers.
 */
[type=file] {
  opacity: 0; /* 1 */
  font-size: 0; /* 2 */
  cursor: pointer; /* 3 */
}

/**
	* Fix appearance for Firefox
	*/
[type=number] {
  -moz-appearance: textfield;
}

/**
 * Set cursor to pointer for all browsers.
 */
[type=range] {
  cursor: pointer;
}

/**
 * Reset slider thumbs to make them styleable.
 */
[type=range]::-webkit-slider-thumb {
  -webkit-appearance: none;
  appearance: none;
}

[type=range]::-moz-range-thumb {
  -moz-appearance: none;
  appearance: none;
  border-width: 0;
  border-radius: 0;
  background-color: transparent;
}

/* Interactive */
/* ============================================ */
/*
 * Add the correct display in Edge, IE 10+, and Firefox.
 */
details {
  display: block;
}

/*
 * Add the correct display in all browsers.
 */
summary {
  display: list-item;
}

/*
 * Remove outline for editable content.
 */
[contenteditable]:focus {
  outline: auto;
}

/* Tables */
/* ============================================ */
/**
1. Correct table border color inheritance in all Chrome and Safari.
*/
table {
  border-color: inherit; /* 1 */
  border-collapse: collapse;
}

caption {
  text-align: left;
}

td,
th {
  vertical-align: top;
  padding: 0;
}

th {
  text-align: left;
  font-weight: inherit;
}

/* Misc */
/* ============================================ */
/*
 * Make placeholder style consistent across all browsers.
 */
::placeholder {
  color: #999;
  opacity: 1;
}

/*
 * Hide focus outline but keep it visible for Windows High Contrast Mode.
 */
:focus {
  outline-style: solid;
  outline-color: transparent;
}

/*
 * Hide input arrow when used with datalist.
 */
::-webkit-calendar-picker-indicator {
  display: none !important;
}`,bw=[ww,$2,Sg,_w,Gx,R2,$c,aw,iw,pw,gw,dw].join(`
`);function kc(e,t="filtered",n){let{name:o,path:r}=Gi(e,n);if(t==="off")return{name:o,elementName:o,path:r,reactComponents:null};let i=Hv(e,{mode:t});return{name:i.path?`${i.path} ${o}`:o,elementName:o,path:r,reactComponents:i.path}}var fg=!1,O_={outputDetail:"standard",autoClearAfterCopy:!1,annotationColorId:"blue",blockInteractions:!0,reactEnabled:!0,markerClickBehavior:"edit",webhookUrl:"",webhooksEnabled:!0},hg=e=>{if(!e||!e.trim())return!1;try{let t=new URL(e.trim());return t.protocol==="http:"||t.protocol==="https:"}catch{return!1}},kw={compact:"off",standard:"filtered",detailed:"smart",forensic:"all"},Cr=e=>e.metaKey||e.ctrlKey,gs=[{id:"indigo",label:"Indigo",srgb:"#6155F5",p3:"color(display-p3 0.38 0.33 0.96)"},{id:"blue",label:"Blue",srgb:"#0088FF",p3:"color(display-p3 0.00 0.53 1.00)"},{id:"cyan",label:"Cyan",srgb:"#00C3D0",p3:"color(display-p3 0.00 0.76 0.82)"},{id:"green",label:"Green",srgb:"#34C759",p3:"color(display-p3 0.20 0.78 0.35)"},{id:"yellow",label:"Yellow",srgb:"#FFCC00",p3:"color(display-p3 1.00 0.80 0.00)"},{id:"orange",label:"Orange",srgb:"#FF8D28",p3:"color(display-p3 1.00 0.55 0.16)"},{id:"red",label:"Red",srgb:"#FF383C",p3:"color(display-p3 1.00 0.22 0.24)"}],Cw=[...gs.map(e=>`
    [data-agentation-accent="${e.id}"] {
      --agentation-color-accent: ${e.srgb};
    }
    @supports (color: color(display-p3 0 0 0)) {
      [data-agentation-accent="${e.id}"] {
        --agentation-color-accent: ${e.p3};
      }
    }
  `),`:host {
    ${gs.map(e=>`--agentation-color-${e.id}: ${e.srgb};`).join(`
`)}
  }`,`@supports (color: color(display-p3 0 0 0)) {
    :host {
      ${gs.map(e=>`--agentation-color-${e.id}: ${e.p3};`).join(`
`)}
    }
  }`].join("");function A_(e){let t=e;for(let o=zn(t.ownerDocument);o;o=zn(t.ownerDocument))t=o;let n=t;for(;n&&n!==document.body;){let r=window.getComputedStyle(n).position;if(r==="fixed"||r==="sticky")return!0;n=n.parentElement}return!1}function hs(e){return e.kind!=="placement"&&e.kind!=="rearrange"&&e.status!=="resolved"&&e.status!=="dismissed"}function Cc(e){let t=V_(e),n=t.found?t:tw(e);if(n.found&&n.source)return ew(n.source,"path")}function Yg(e={}){let t=a2(e.useHashLocation??!1),n=(0,k.useState)(!1),o=u2(e.portalContainer);return o?(0,kg.createPortal)((0,Ug.createElement)(Sw,{...e,key:e.useHashLocation?t:void 0,pathname:t,activeState:n,portalHost:o}),o):null}function Sw({pathname:e,activeState:t,portalHost:n,useHashLocation:o=!1,appName:r,enableKeyboardShortcuts:i=!0,identifyingAttributes:l=G_,copyFormat:s="markdown",onOpenSource:a,portalContainer:c,demoAnnotations:f,demoDelay:u=1e3,enableDemoMode:x=!1,onAnnotationAdd:S,onAnnotationDelete:b,onAnnotationUpdate:N,onAnnotationsClear:E,onCopy:g,onSubmit:v,copyToClipboard:h=!0,endpoint:C,sessionId:U,onSessionCreated:V,webhookUrl:B,className:q}){let[F,K]=t,[,se]=(0,k.useState)(0),[Z]=(0,k.useState)(()=>K5(document,()=>se(d=>d+1)));(0,k.useEffect)(()=>(Z.start(),()=>Z.stop()),[Z]);let fe=typeof s=="object"?s.attribute:void 0,oe=(0,k.useMemo)(()=>fe?[...l,fe]:l,[l,fe]),ae=(0,k.useRef)(!0);(0,k.useLayoutEffect)(()=>(ae.current=!0,()=>{ae.current=!1}),[]);let pe=(0,k.useCallback)(d=>o?c2(JSON.stringify([C,e]),d):d(),[C,e,o]),kt=(0,k.useRef)(new Map),Oe=(0,k.useRef)(new Set),qe=d=>hs(d)&&!Oe.current.has(d.id),Ae=async(...d)=>{let p=await eg(...d),w=d[2],M=w.id;if(M&&(kt.current.set(M,p.id),Oe.current.has(M)&&Oe.current.add(p.id)),!o){let $=new URL(w.url||window.location.href).pathname,I=kr($).find(T=>T.id===M);try{if(Oe.current.has(M))await xc(d[0],p.id);else if(I&&I.comment!==w.comment)return await $_(d[0],p.id,{comment:I.comment}),{...p,comment:I.comment}}catch(T){console.warn("[Agentation] Failed to apply changes made during sync:",T)}}return p},_t=d=>o?{...d,annotations:d.annotations.filter(p=>!Oe.current.has(p.id)&&d2(p.url||d.url,e,window.location.origin))}:d,qt=(0,k.useRef)(e);qt.current=e;let wn=(d,p,w,M=e)=>{let $=Y5(d,kr(M),p,kt.current).filter(qe);M===qt.current&&ae.current&&Zn($),N_(M,$,w)},[Re,Zn]=(0,k.useState)([]),[Ro,qo]=(0,k.useState)(!0),[Ko,$o]=(0,k.useState)(()=>Nv()),[To,Po]=(0,k.useState)(!1);(0,k.useLayoutEffect)(()=>{mg()},[]);let Bt=(0,k.useRef)(null),eo=(0,k.useRef)(null),mo=(0,k.useRef)(null),Er=(0,k.useRef)(null),On=(0,k.useRef)(!1),Do=(0,k.useRef)(!1),G=(0,k.useRef)(!1);(0,k.useLayoutEffect)(()=>{F&&On.current?(On.current=!1,mo.current?.querySelector("button:not(:disabled)")?.focus()):!F&&Do.current&&(Do.current=!1,eo.current?.focus())},[F]),(0,k.useEffect)(()=>{let d=w=>{let M=Bt.current;M&&w.composedPath().includes(M)&&w.stopPropagation()},p=["mousedown","click","pointerdown"];return p.forEach(w=>n.addEventListener(w,d)),()=>{p.forEach(w=>n.removeEventListener(w,d))}},[n]);let[he,Ne]=(0,k.useState)(!1),[Me,be]=(0,k.useState)(!1),[Ke,We]=(0,k.useState)(null),[Qe,Ye]=(0,k.useState)({x:0,y:0}),[H,L]=(0,k.useState)(null),[R,z]=(0,k.useState)(!1),j=L0(),ee=L0(),[te,Y]=(0,k.useState)("idle"),[de,ke]=(0,k.useState)(!1),Te=(0,k.useRef)(new Set),rt=(0,k.useRef)(new Set),st=(0,k.useRef)(),Je=(0,k.useCallback)(()=>{!Te.current.size&&!st.current&&ke(!1)},[]);(0,k.useEffect)(()=>()=>clearTimeout(st.current),[]);let[je,Ie]=(0,k.useState)(null),[gt,mt]=(0,k.useState)(null),[at,Pe]=(0,k.useState)([]),[Ze,ft]=(0,k.useState)(null),ct=(0,k.useRef)(null);(0,k.useEffect)(()=>()=>{ct.current&&clearTimeout(ct.current)},[]);let[ie,In]=(0,k.useState)(null),bn=(0,k.useRef)(null),rn=(0,k.useRef)(!1),[go,Bo]=(0,k.useState)(!1);(0,k.useLayoutEffect)(()=>{if(ie||!bn.current)return;let d=bn.current;if(bn.current=null,H)return;(F&&d.isConnected&&!d.disabled?d:eo.current)?.focus({preventScroll:!0})},[ie,F,H]);let[to,An]=(0,k.useState)(null),[tl,nl]=(0,k.useState)([]),[Lr,rf]=(0,k.useState)(0),[lf,sf]=(0,k.useState)(!1),[it,Xg]=(0,k.useState)(!1),[Fn,af]=(0,k.useState)(!1),[ln,Nr]=(0,k.useState)(!1),[Gg,cf]=(0,k.useState)("main"),[df,Pc]=(0,k.useState)(!1),[Ve,Dc]=(0,k.useState)(!1),[Ir,ol]=(0,k.useState)(!1),[ze,zo]=(0,k.useState)([]),[si,Rr]=(0,k.useState)(null),Bc=(0,k.useRef)(!1),[yt,uf]=(0,k.useState)(!1),[_f,zc]=(0,k.useState)(!1),[ff,qg]=(0,k.useState)(1),[hf,Mw]=(0,k.useState)("new-page"),[sn,ws]=(0,k.useState)(""),[Kg,Jg]=(0,k.useState)(!1),[me,yo]=(0,k.useState)(null),Oc=(0,k.useRef)(!1),Ac=(0,k.useRef)({rearrange:null,placements:[]}),$r=(0,k.useRef)({rearrange:null,placements:[]}),[Zg,pf]=(0,k.useState)(0),[e1,t1]=(0,k.useState)(0),[ai,mf]=(0,k.useState)([]),[ci,gf]=(0,k.useState)(null),yf=(0,k.useRef)({designPlacements:ze,rearrangeState:me,blankCanvas:yt,wireframePurpose:sn});yf.current={designPlacements:ze,rearrangeState:me,blankCanvas:yt,wireframePurpose:sn};let di=(0,k.useRef)({placements:ai,rearrange:ci}),rl=(0,k.useRef)(new Set),bs=(0,k.useRef)(new Set),no=(0,k.useRef)(null),ui=(0,k.useRef)(),xf=Ve&&F&&!Ir&&yt;(0,k.useEffect)(()=>{if(xf){zc(!1);let d=Ec(()=>{zc(!0)});return()=>cancelAnimationFrame(d)}else zc(!1)},[xf]);let Fc=(0,k.useRef)(new Map),Wc=(0,k.useRef)([]),jc=(0,k.useRef)(new Map),Tr=(0,k.useRef)(null),[Wn,Hc]=(0,k.useState)(!1),[oo,n1]=(0,k.useState)([]),Uc=(0,k.useRef)(oo);Uc.current=oo;let[vf,Ew]=(0,k.useState)(null),Yc=(0,k.useRef)(null),Lw=(0,k.useRef)(!1),Nw=(0,k.useRef)([]),Iw=(0,k.useRef)(0),Rw=(0,k.useRef)(null),$w=(0,k.useRef)(null),Tw=(0,k.useRef)(1),[Qc,wf]=(0,k.useState)(!1),_i=(0,k.useRef)(null),[kn,Jo]=(0,k.useState)([]),il=(0,k.useRef)(!1),an=()=>{Pc(!0)},o1=()=>{Pc(!1)},bf=()=>{Qc||(_i.current=nt(()=>wf(!0),850))},kf=()=>{_i.current&&(clearTimeout(_i.current),_i.current=null),wf(!1),o1()};(0,k.useEffect)(()=>()=>{_i.current&&clearTimeout(_i.current)},[]);let[et,r1]=(0,k.useState)(()=>{try{let d=JSON.parse(localStorage.getItem("feedback-toolbar-settings")??"");return{...O_,...d,annotationColorId:gs.find(p=>p.id===d.annotationColorId)?d.annotationColorId:O_.annotationColorId}}catch{return O_}}),[xo,Cf]=(0,k.useState)(!0),[Sf,Mf]=(0,k.useState)(!1),i1=(0,k.useCallback)(d=>{r1(p=>({...p,...d}))},[]),l1=(0,k.useCallback)(()=>{Bt.current?.classList.add(O.disableTransitions),Cf(d=>!d),Ec(()=>{Bt.current?.classList.remove(O.disableTransitions)})},[]),Ef=!1,Oo=Ef&&et.reactEnabled?kw[et.outputDetail]:"off",[jt,Vc]=(0,k.useState)(o?null:U??null),Lf=(0,k.useRef)(!1),[Pr,Dr]=(0,k.useState)(C?"connecting":"disconnected"),[Tt,Xc]=(0,k.useState)(null),[ks,Nf]=(0,k.useState)(!1),ll=(0,k.useRef)(null),Cs=(0,k.useRef)(!1),Br=(0,k.useRef)(new Set),Ss=(0,k.useRef)(new Map),If=(0,k.useCallback)(d=>{Br.current.add(d),ro.current===d&&(ro.current=null)},[]),[Zo,Ms]=(0,k.useState)(new Set),[jn,sl]=(0,k.useState)(!1),[zr,fi]=(0,k.useState)(!1),[Ao,Gc]=(0,k.useState)(!1),Or=(0,k.useRef)(null),Hn=(0,k.useRef)(null),al=(0,k.useRef)(null),hi=(0,k.useRef)(null),cl=(0,k.useRef)(!1),Rf=(0,k.useRef)(0),ro=(0,k.useRef)(null),$f=(0,k.useRef)(null),qc=8,s1=50,Kc=(0,k.useRef)(null),Es=(0,k.useRef)(null),dl=(0,k.useRef)(null),a1=(0,k.useCallback)(()=>cf("main"),[]);(0,k.useEffect)(()=>{ln||Pc(!1)},[ln]),(0,k.useLayoutEffect)(()=>{ln&&G.current&&(G.current=!1,Bt.current?.querySelector("[data-agentation-settings-panel] button")?.focus())},[ln]);let Ls=F&&Ro&&!Ve;(0,k.useEffect)(()=>{if(Ls)be(!1),Ne(!0),Br.current.clear();else if(he){be(!0);let d=nt(()=>{Ne(!1),be(!1)},250);return()=>clearTimeout(d)}},[Ls]),(0,k.useEffect)(()=>{Xg(!0),rf(window.scrollY);let d=kr(e);Zn(d.filter(hs)),fg||(Mf(!0),fg=!0,nt(()=>Mf(!1),750));try{let p=localStorage.getItem("feedback-toolbar-theme");p!==null&&Cf(p==="dark")}catch{}try{let p=localStorage.getItem("feedback-toolbar-position");if(p){let w=JSON.parse(p);typeof w.x=="number"&&typeof w.y=="number"&&Xc(w)}}catch{}},[e]),(0,k.useEffect)(()=>{it&&localStorage.setItem("feedback-toolbar-settings",JSON.stringify(et))},[et,it]),(0,k.useEffect)(()=>{it&&localStorage.setItem("feedback-toolbar-theme",xo?"dark":"light")},[xo,it]);let Tf=(0,k.useRef)(!1);(0,k.useEffect)(()=>{let d=Tf.current;Tf.current=ks,d&&!ks&&Tt&&it&&localStorage.setItem("feedback-toolbar-position",JSON.stringify(Tt))},[ks,Tt,it]),(0,k.useEffect)(()=>{if(!C||!it||Lf.current)return;Lf.current=!0,Dr("connecting");let d=window.location.href;pe(async()=>{try{let w=Ev(e),M=U||w,$=!1;if(M)try{let I=kr(e),T=_t(await Z0(C,M));Wc.current=T.annotations.filter(ge=>ge.kind==="placement"||ge.kind==="rearrange"),ae.current&&(Vc(T.id),Dr("connected")),I_(e,T.id),$=!0;let X=kr(e).filter(hs),_e=new Set(T.annotations.map(ge=>ge.id)),ye=X.filter(ge=>!_e.has(ge.id));if(ye.length>0){let xe=`${typeof window<"u"?window.location.origin:""}${e}`,Fe=(await Promise.allSettled(ye.map(we=>Ae(C,T.id,{...we,sessionId:T.id,url:xe})))).map((we,Ee)=>we.status==="fulfilled"?we.value:(console.warn("[Agentation] Failed to sync annotation:",we.reason),ye[Ee])),ue=[...T.annotations,...Fe];wn(I,ue,T.id)}else wn(I,T.annotations,T.id)}catch(I){console.warn("[Agentation] Could not join session, creating new:",I),Lv(e)}if(!$){let I=await R_(C,d);I_(e,I.id),ae.current&&(Vc(I.id),Dr("connected"),V?.(I.id));let T=o?new Map([[e,kr(e)]]):xv(),X=typeof window<"u"?window.location.origin:"",_e=[];for(let[ye,ge]of T){let xe=ge.filter(ue=>hs(ue)&&!ue._syncedTo);if(xe.length===0)continue;let $e=`${X}${ye}`,Fe=ye===e;_e.push((async()=>{try{let ue=Fe?I:await R_(C,$e),Ee=(await Promise.allSettled(xe.map(Xe=>Ae(C,ue.id,{...Xe,sessionId:ue.id,url:$e})))).map((Xe,Ct)=>Xe.status==="fulfilled"?Xe.value:(console.warn("[Agentation] Failed to sync annotation:",Xe.reason),xe[Ct]));wn(xe,Ee,ue.id,ye)}catch(ue){console.warn(`[Agentation] Failed to sync annotations for ${ye}:`,ue)}})())}await Promise.allSettled(_e)}}catch(w){ae.current&&Dr("disconnected"),console.warn("[Agentation] Failed to initialize session, using local storage:",w)}})},[C,U,it,V,e,pe]),(0,k.useEffect)(()=>{if(!C||!it)return;let d=async()=>{try{(await fetch(`${C}/health`)).ok?Dr("connected"):Dr("disconnected")}catch{Dr("disconnected")}};d();let p=gg(d,1e4);return()=>clearInterval(p)},[C,it]);let Ns=(0,k.useRef)(Re),Pf=(0,k.useRef)(!1);(0,k.useLayoutEffect)(()=>{Ns.current=Re,Pf.current=Re.length>0||ze.length>0||(me?.sections.length??0)>0},[Re,ze.length,me?.sections.length]);let Jc=(0,k.useCallback)(d=>{let p=Te.current.has(d);if(p&&(rt.current.delete(d),rt.current.size))return;let w=p?new Set(Te.current):new Set([d]);p&&(Te.current.clear(),Je());for(let I of w)Ss.current.delete(I),Br.current.delete(I);Zn(I=>I.filter(T=>!w.has(T.id))),Ms(I=>new Set([...I].filter(T=>!w.has(T))));let M=p?[]:Ns.current.filter(I=>I.kind!=="placement"&&I.kind!=="rearrange"),$=M.findIndex(I=>I.id===d);$>=0&&$<M.length-1&&(ft(I=>I===null?$:Math.min(I,$)),ct.current&&clearTimeout(ct.current),ct.current=nt(()=>ft(null),200))},[Je]);(0,k.useEffect)(()=>!C||!it||!jt?void 0:rw(C,jt,()=>Pf.current,w=>{let{id:M,kind:$}=w;if($==="placement"){for(let[I,T]of Fc.current)if(T===M){Tr.current?.placements.forget(I),zo(X=>X.filter(_e=>_e.id!==I));break}}else if($==="rearrange"){for(let[I,T]of jc.current)if(T===M){Tr.current?.rearrange.forget(I),yo(X=>{if(!X)return null;let _e=X.sections.filter(ye=>ye.id!==I);return _e.length===0?null:{...X,sections:_e}});break}}else{if(!Ns.current.some(I=>I.id===M))return;Ms(I=>new Set(I).add(M))}}),[C,it,jt]),(0,k.useEffect)(()=>{if(!C||!it)return;let d=$f.current==="disconnected",p=Pr==="connected";$f.current=Pr,d&&p&&pe(async()=>{try{let M=kr(e).filter(hs);if(M.length===0)return;let I=`${typeof window<"u"?window.location.origin:""}${e}`,T=jt,X=[];if(T)try{X=_t(await Z0(C,T)).annotations}catch{T=null}T||(T=(await R_(C,I)).id,ae.current&&Vc(T),I_(e,T));let _e=new Set(X.map(ge=>ge.id)),ye=M.filter(ge=>!_e.has(ge.id));if(ye.length>0){let xe=(await Promise.allSettled(ye.map(Fe=>Ae(C,T,{...Fe,sessionId:T,url:I})))).map((Fe,ue)=>Fe.status==="fulfilled"?Fe.value:(console.warn("[Agentation] Failed to sync annotation on reconnect:",Fe.reason),ye[ue])),$e=[...X,...xe];wn(M,$e,T)}}catch(M){console.warn("[Agentation] Failed to sync on reconnect:",M)}})},[Pr,C,it,jt,e,pe]);let c1=(0,k.useCallback)(()=>{To||(Po(!0),Nr(!1),K(!1),nt(()=>{Iv(!0),$o(!0),Po(!1)},400))},[To]);(0,k.useEffect)(()=>{if(!x||!it||!f||f.length===0||Re.length>0)return;let d=[];return d.push(nt(()=>{K(!0)},u-200)),f.forEach((p,w)=>{let M=u+w*300;d.push(nt(()=>{let $=document.querySelector(p.selector);if(!$)return;let I=Wt($),{name:T,path:X}=Gi($),_e={id:`demo-${Date.now()}-${w}`,x:(I.left+I.width/2)/window.innerWidth*100,y:I.top+I.height/2+window.scrollY,comment:p.comment,element:T,elementPath:X,timestamp:Date.now(),selectedText:p.selectedText,boundingBox:{x:I.left,y:I.top+window.scrollY,width:I.width,height:I.height},nearbyText:cs($),cssClasses:ds($)};Zn(ye=>[...ye,_e])},M))}),()=>{d.forEach(clearTimeout)}},[x,it,f,u]),(0,k.useEffect)(()=>{let d=()=>{rf(window.scrollY),se(p=>p+1),sf(!0),dl.current&&clearTimeout(dl.current),dl.current=nt(()=>{sf(!1)},150)};return Z.addEventListener("scroll",d,{passive:!0,capture:!0}),()=>{Z.removeEventListener("scroll",d,!0),dl.current&&clearTimeout(dl.current)}},[Z]),(0,k.useEffect)(()=>{if(!it)return;let d=Re.filter(p=>!Zo.has(p.id));d.length>0?jt?N_(e,d,jt):Wg(e,d):localStorage.removeItem(Z_(e))},[Re,e,it,jt,de,Zo]),(0,k.useEffect)(()=>{if(it&&!Bc.current){Bc.current=!0;let d=vv(e);d.length>0&&zo(d)}},[it,e]),(0,k.useEffect)(()=>{if(it&&Bc.current&&!yt){let d=ze.filter(p=>!ai.includes(p));d.length>0?wv(e,d):bv(e)}},[ze,e,it,yt,ai]),(0,k.useEffect)(()=>{if(it&&!Oc.current){Oc.current=!0;let d=kv(e);if(d){let p={...d,sections:d.sections.map(w=>({...w,currentRect:w.currentRect??{...w.originalRect}}))};yo(p)}}},[it,e]),(0,k.useEffect)(()=>{it&&Oc.current&&!yt&&(me&&me!==ci?Cv(e,me):Sv(e))},[me,e,it,yt,ci]);let Zc=(0,k.useRef)(!1);(0,k.useEffect)(()=>{if(it&&!Zc.current){Zc.current=!0;let d=Mv(e);d&&($r.current={rearrange:d.rearrange,placements:d.placements||[]},d.purpose&&ws(d.purpose))}},[it,e]),(0,k.useEffect)(()=>{if(!it||!Zc.current||de)return;let d=$r.current;yt?(me?.sections?.length??0)>0||ze.length>0||sn?J0(e,{rearrange:me,placements:ze,purpose:sn}):yc(e):(d.rearrange?.sections?.length??0)>0||d.placements.length>0||sn?J0(e,{rearrange:d.rearrange,placements:d.placements,purpose:sn}):yc(e)},[me,ze,sn,yt,e,it,de]),(0,k.useEffect)(()=>{Ve&&!me&&yo({sections:[],originalOrder:[],detectedAt:Date.now()})},[Ve,me]),(0,k.useEffect)(()=>{if(!C||!jt)return;let d={create:w=>pe(()=>eg(C,jt,w)),update:(w,M)=>pe(()=>$_(C,w,M)),remove:w=>pe(()=>xc(C,w))};Fc.current=new Map,jc.current=new Map;let p={placements:lg(d,Fc.current,Wc.current.filter(w=>w.kind==="placement")),rearrange:lg(d,jc.current,Wc.current.filter(w=>w.kind==="rearrange"))};return Tr.current=p,()=>{p.placements.dispose(),p.rearrange.dispose(),Tr.current===p&&(Tr.current=null)}},[C,jt,e,pe]),(0,k.useEffect)(()=>{let d=window.location.pathname+window.location.search+window.location.hash;Tr.current?.placements.replace(ze.filter(p=>!ai.includes(p)).map(p=>({id:p.id,x:p.x/window.innerWidth*100,y:p.y,comment:`Place ${p.type} at (${Math.round(p.x)}, ${Math.round(p.y)}), ${p.width}\xD7${p.height}px${p.text?` \u2014 "${p.text}"`:""}`,element:`[design:${p.type}]`,elementPath:"[placement]",timestamp:p.timestamp,url:d,intent:"change",severity:"important",kind:"placement",placement:{componentType:p.type,width:p.width,height:p.height,scrollY:p.scrollY,text:p.text}})))},[ze,C,jt,e,ai]),(0,k.useEffect)(()=>{let d=Tr.current;if(!d)return;if(me===ci){d.rearrange.replace([]);return}let p=nt(()=>{let w=window.location.pathname+window.location.search+window.location.hash,M=[];for(let $ of me?.sections??[]){let I=$.originalRect,T=$.currentRect,X=Math.abs(I.x-T.x)>1||Math.abs(I.y-T.y)>1||Math.abs(I.width-T.width)>1||Math.abs(I.height-T.height)>1;if(!X&&!$.note)continue;let _e=$.note?` \u2014 "${$.note}"`:"";M.push({id:$.id,x:T.x/window.innerWidth*100,y:T.y,comment:X?`Move ${$.label} section (${$.tagName}) \u2014 from (${Math.round(I.x)},${Math.round(I.y)}) ${Math.round(I.width)}\xD7${Math.round(I.height)} to (${Math.round(T.x)},${Math.round(T.y)}) ${Math.round(T.width)}\xD7${Math.round(T.height)}${_e}`:`Note on ${$.label} section (${$.tagName})${_e}`,element:$.selector,elementPath:"[rearrange]",timestamp:me.detectedAt,url:w,intent:"change",severity:"important",kind:"rearrange",rearrange:{selector:$.selector,label:$.label,tagName:$.tagName,originalRect:I,currentRect:T}})}d.rearrange.replace(M)},300);return()=>clearTimeout(p)},[me,C,jt,e,ci]);let ed=(0,k.useCallback)(()=>{clearTimeout(ui.current),ol(!1),Dc(!0)},[]);(0,k.useEffect)(()=>()=>clearTimeout(ui.current),[]);let ul=(0,k.useCallback)(()=>{ol(!0),Dc(!1),Rr(null),clearTimeout(ui.current),ui.current=nt(()=>{ol(!1)},300)},[]),Is=(0,k.useCallback)(()=>{let d=eo.current?.getRootNode();Do.current=!!d?.activeElement&&!!Bt.current?.contains(d.activeElement),Do.current&&d?.activeElement?.blur(),Nr(!1),Ve&&(ol(!0),Dc(!1),Rr(null),clearTimeout(ui.current),ui.current=nt(()=>{ol(!1)},300)),K(!1)},[Ve]),Df=(0,k.useCallback)(()=>{Fn||(U5(),af(!0))},[Fn]),Rs=(0,k.useCallback)(()=>{Fn&&(E0(),af(!1))},[Fn]),td=(0,k.useCallback)(()=>{Fn?Rs():Df()},[Fn,Df,Rs]),_l=(0,k.useCallback)((d=kn)=>{let p=d.filter(T=>T.element.isConnected);if(p.length===0){Jo([]);return}let w=p[0],M=w.element,$=p.length>1,I=p.map(T=>Wt(T.element));if($){let T={left:Math.min(...I.map(Ee=>Ee.left)),top:Math.min(...I.map(Ee=>Ee.top)),right:Math.max(...I.map(Ee=>Ee.right)),bottom:Math.max(...I.map(Ee=>Ee.bottom))},X=p.slice(0,5).map(Ee=>Ee.name).join(", "),_e=p.length>5?` +${p.length-5} more`:"",ye=I.map(Ee=>({x:Ee.left,y:Ee.top+window.scrollY,width:Ee.width,height:Ee.height})),xe=p[p.length-1].element,$e=I[I.length-1],Fe=$e.left+$e.width/2,ue=$e.top+$e.height/2,we=A_(xe);L({id:Date.now().toString(),x:Fe/window.innerWidth*100,y:we?ue:ue+window.scrollY,clientY:ue,element:`${p.length} elements: ${X}${_e}`,elementPath:"multi-select",boundingBox:{x:T.left,y:T.top+window.scrollY,width:T.right-T.left,height:T.bottom-T.top},isMultiSelect:!0,isFixed:we,elementBoundingBoxes:ye,multiSelectElements:p.map(Ee=>Ee.element),targetElement:xe,fullPath:ms(M),accessibility:uc(M),computedStyles:dc(M),computedStylesObj:cc(M),nearbyElements:ac(M),cssClasses:ds(M),nearbyText:cs(M),sourceFile:Cc(M),attributes:ps(M,oe)})}else{let T=I[0],X=A_(M);L({id:Date.now().toString(),x:T.left/window.innerWidth*100,y:X?T.top:T.top+window.scrollY,clientY:T.top,element:w.name,elementPath:w.path,boundingBox:{x:T.left,y:X?T.top:T.top+window.scrollY,width:T.width,height:T.height},isFixed:X,fullPath:ms(M),accessibility:uc(M),computedStyles:dc(M),computedStylesObj:cc(M),nearbyElements:ac(M),cssClasses:ds(M),nearbyText:cs(M),reactComponents:w.reactComponents,targetElement:M,sourceFile:Cc(M),attributes:ps(M,oe)})}Jo([]),We(null)},[kn,oe]);(0,k.useEffect)(()=>{F||(L(null),In(null),An(null),nl([]),We(null),Nr(!1),Jo([]),il.current=!1,Fn&&Rs())},[F,Fn,Rs]),(0,k.useEffect)(()=>()=>{E0()},[]),(0,k.useEffect)(()=>{if(!F)return;let d=["p","span","h1","h2","h3","h4","h5","h6","li","td","th","label","blockquote","figcaption","caption","legend","dt","dd","pre","code","em","strong","b","i","u","s","a","time","address","cite","q","abbr","dfn","mark","small","sub","sup","[contenteditable]"].join(", "),p=document.createElement("style");return p.id="agentation-cursor",p.textContent=`
      body { cursor: crosshair !important; }
      body :is(${d}) { cursor: text !important; }
    `,document.head.appendChild(p),()=>{let w=document.getElementById("agentation-cursor");w&&w.remove()}},[F]),(0,k.useEffect)(()=>{if(vf!==null&&F)return document.documentElement.setAttribute("data-drawing-hover",""),()=>document.documentElement.removeAttribute("data-drawing-hover")},[vf,F]),(0,k.useEffect)(()=>{if(!F||H||ie||Wn||Ve)return;let d=null,p=(I,T,X)=>{let _e=Sc(I,T),ye=X?I0(I,T):_e;if(!ye||nn(ye,"[data-feedback-toolbar], [data-annotation-popup], [data-annotation-marker]")){We(null);return}let{name:ge,elementName:xe,path:$e,reactComponents:Fe}=kc(ye,Oo,oe);We({element:ge,elementName:xe,elementPath:$e,rect:Wt(ye),reactComponents:Fe,isPiercing:X&&ye!==_e}),Ye({x:I,y:T})},w=I=>{let T=I.composedPath()[0]||I.target;if(nn(T,"[data-feedback-toolbar], [data-annotation-popup], [data-annotation-marker]")){d=null,We(null);return}d={x:I.clientX,y:I.clientY},p(I.clientX,I.clientY,Cr(I))},M=I=>{(I.key==="Meta"||I.key==="Control")&&d&&p(d.x,d.y,Cr(I))},$=()=>{d=null,We(null)};return Z.addEventListener("mousemove",w),Z.addEventListener("keydown",M),Z.addEventListener("keyup",M),Z.addEventListener("mouseleave",$),window.addEventListener("blur",$),()=>{Z.removeEventListener("mousemove",w),Z.removeEventListener("keydown",M),Z.removeEventListener("keyup",M),Z.removeEventListener("mouseleave",$),window.removeEventListener("blur",$)}},[F,H,ie,Wn,Ve,Oo,oe]);let $s=(0,k.useCallback)((d,p)=>{if(ie&&!zr){Es.current?.shake();return}if(H&&!jn){if(Bt.current?.querySelector("[data-annotation-popup]:not([data-annotation-card]) textarea")?.value.trim()){Kc.current?.shake();return}sl(!0)}if(bn.current=p??null,rn.current=p?.matches(":focus-visible")??!1,Bo(!1),fi(!1),In(d),Ie(null),mt(null),Pe([]),d.elementBoundingBoxes?.length){let w=[];for(let M of d.elementBoundingBoxes){let $=M.x+M.width/2,I=M.y+M.height/2-window.scrollY,T=_c($,I,M);T&&w.push(T)}nl(w),An(null)}else if(d.boundingBox){let w=d.boundingBox,M=w.x+w.width/2,$=d.isFixed?w.y+w.height/2:w.y+w.height/2-window.scrollY,I=_c(M,$,w);if(I){let T=Wt(I),X=T.width/w.width,_e=T.height/w.height;X<.5||_e<.5?An(null):An(I)}else An(null);nl([])}else An(null),nl([])},[H,jn,ie,zr]);(0,k.useEffect)(()=>{if(!F||Wn||Ve)return;let d=p=>{if(cl.current){cl.current=!1,p.preventDefault(),p.stopPropagation();return}let w=p.composedPath()[0]||p.target;if(nn(w,"[data-feedback-toolbar]")||nn(w,"[data-annotation-popup]")||nn(w,"[data-annotation-marker]"))return;if(Cr(p)&&!H&&!ie){p.preventDefault(),p.stopPropagation(),il.current=p.shiftKey;let Ee=I0(p.clientX,p.clientY);if(!Ee)return;let Xe=Wt(Ee),{name:Ct,path:pn,reactComponents:Le}=kc(Ee,Oo,oe),Ce=kn.findIndex(tt=>tt.element===Ee);Ce>=0?Jo(tt=>tt.filter((Pt,bo)=>bo!==Ce)):Jo(tt=>[...tt,{element:Ee,rect:Xe,name:Ct,path:pn,reactComponents:Le??void 0}]);return}let M=nn(w,"button, a, input, select, textarea, [role='button'], [onclick]");if(et.blockInteractions&&(p.preventDefault(),p.stopPropagation()),H&&!jn){if(M&&!et.blockInteractions)return;p.preventDefault(),Kc.current?.shake();return}if(ie&&!zr){if(M&&!et.blockInteractions)return;p.preventDefault(),Es.current?.shake();return}p.preventDefault();let $=Sc(p.clientX,p.clientY);if(!$)return;let{name:I,path:T,reactComponents:X}=kc($,Oo,oe),_e=Wt($),ye=p.clientX/window.innerWidth*100,ge=A_($),xe=ge?p.clientY:p.clientY+window.scrollY,$e=$.ownerDocument.defaultView?.getSelection(),Fe;$e&&$e.toString().trim().length>0&&(Fe=$e.toString().trim().slice(0,500));let ue=cc($),we=dc($);sl(!1),L({id:Date.now().toString(),x:ye,y:xe,clientY:p.clientY,element:I,elementPath:T,selectedText:Fe,boundingBox:{x:_e.left,y:ge?_e.top:_e.top+window.scrollY,width:_e.width,height:_e.height},nearbyText:cs($),cssClasses:ds($),isFixed:ge,fullPath:ms($),accessibility:uc($),computedStyles:we,computedStylesObj:ue,nearbyElements:ac($),reactComponents:X??void 0,sourceFile:Cc($),attributes:ps($,oe),frame:V5($,p.clientX,p.clientY),targetElement:$}),We(null)};return Z.addEventListener("click",d,!0),()=>Z.removeEventListener("click",d,!0)},[F,Wn,Ve,H,jn,ie,zr,et.blockInteractions,Oo,oe,kn]),(0,k.useEffect)(()=>{if(!F)return;let d=w=>{let M=(w.key==="Meta"||w.key==="Control")&&!Cr(w),$=w.key==="Shift"&&il.current;(M||$)&&!Hn.current&&kn.length>0&&_l()},p=()=>{il.current=!1,Jo([]),We(null),Or.current=null,Hn.current=null,Gc(!1),hi.current?.replaceChildren()};return Z.addEventListener("keyup",d),window.addEventListener("blur",p),()=>{Z.removeEventListener("keyup",d),window.removeEventListener("blur",p)}},[F,kn,_l]),(0,k.useEffect)(()=>{if(!F||H||Wn||Ve)return;let d=p=>{if(p.button!==0)return;cl.current=!1;let w=p.composedPath()[0]||p.target;if(nn(w,"[data-feedback-toolbar]")||nn(w,"[data-annotation-marker]")||nn(w,"[data-annotation-popup]"))return;let M=new Set(["P","SPAN","H1","H2","H3","H4","H5","H6","LI","TD","TH","LABEL","BLOCKQUOTE","FIGCAPTION","CAPTION","LEGEND","DT","DD","PRE","CODE","EM","STRONG","B","I","U","S","A","TIME","ADDRESS","CITE","Q","ABBR","DFN","MARK","SMALL","SUB","SUP"]);!Cr(p)&&(M.has(w.tagName)||w.isContentEditable)||(p.preventDefault(),Or.current={x:p.clientX,y:p.clientY})};return Z.addEventListener("mousedown",d),()=>Z.removeEventListener("mousedown",d)},[F,H,Wn,Ve]),(0,k.useEffect)(()=>{if(!F||H)return;let d=p=>{if(!Or.current)return;let w=p.clientX-Or.current.x,M=p.clientY-Or.current.y,$=w*w+M*M,I=qc*qc;if(!Ao&&$>=I&&(Hn.current=Or.current,Gc(!0),p.preventDefault()),(Ao||$>=I)&&Hn.current){if(al.current){let Le=Math.min(Hn.current.x,p.clientX),Ce=Math.min(Hn.current.y,p.clientY),tt=Math.abs(p.clientX-Hn.current.x),Pt=Math.abs(p.clientY-Hn.current.y);al.current.style.transform=`translate(${Le}px, ${Ce}px)`,al.current.style.width=`${tt}px`,al.current.style.height=`${Pt}px`}let T=Date.now();if(T-Rf.current<s1)return;Rf.current=T;let X=Hn.current.x,_e=Hn.current.y,ye=Math.min(X,p.clientX),ge=Math.min(_e,p.clientY),xe=Math.max(X,p.clientX),$e=Math.max(_e,p.clientY),Fe=(ye+xe)/2,ue=(ge+$e)/2,we=new Set,Ee=[[ye,ge],[xe,ge],[ye,$e],[xe,$e],[Fe,ue],[Fe,ge],[Fe,$e],[ye,ue],[xe,ue]];for(let[Le,Ce]of Ee){let tt=document.elementsFromPoint(Le,Ce);for(let Pt of tt)Pt instanceof HTMLElement&&we.add(Pt)}let Xe=Z.querySelectorAll("button, a, input, img, p, h1, h2, h3, h4, h5, h6, li, label, td, th, div, span, section, article, aside, nav");for(let Le of Xe)if(Le instanceof HTMLElement){let Ce=Wt(Le),tt=Ce.left+Ce.width/2,Pt=Ce.top+Ce.height/2,bo=tt>=ye&&tt<=xe&&Pt>=ge&&Pt<=$e,io=Math.min(Ce.right,xe)-Math.max(Ce.left,ye),hl=Math.min(Ce.bottom,$e)-Math.max(Ce.top,ge),Os=io>0&&hl>0?io*hl:0,Kt=Ce.width*Ce.height,pl=Kt>0?Os/Kt:0;(bo||pl>.5)&&we.add(Le)}let Ct=[],pn=new Set(["BUTTON","A","INPUT","IMG","P","H1","H2","H3","H4","H5","H6","LI","LABEL","TD","TH","SECTION","ARTICLE","ASIDE","NAV"]);for(let Le of we){if(nn(Le,"[data-feedback-toolbar]")||nn(Le,"[data-annotation-marker]"))continue;let Ce=Wt(Le);if(!(Ce.width>window.innerWidth*.8&&Ce.height>window.innerHeight*.5)&&!(Ce.width<10||Ce.height<10)&&Ce.left<xe&&Ce.right>ye&&Ce.top<$e&&Ce.bottom>ge){let tt=Le.tagName,Pt=pn.has(tt);if(!Pt&&(tt==="DIV"||tt==="SPAN")){let bo=Le.textContent&&Le.textContent.trim().length>0,io=Le.onclick!==null||Le.getAttribute("role")==="button"||Le.getAttribute("role")==="link"||Le.classList.contains("clickable")||Le.hasAttribute("data-clickable");(bo||io)&&!Le.querySelector("p, h1, h2, h3, h4, h5, h6, button, a")&&(Pt=!0)}if(Pt){let bo=!1;for(let io of Ct)if(io.left<=Ce.left&&io.right>=Ce.right&&io.top<=Ce.top&&io.bottom>=Ce.bottom){bo=!0;break}bo||Ct.push(Ce)}}}if(hi.current){let Le=hi.current;for(;Le.children.length>Ct.length;)Le.removeChild(Le.lastChild);Ct.forEach((Ce,tt)=>{let Pt=Le.children[tt];Pt||(Pt=document.createElement("div"),Pt.className=O.selectedElementHighlight,Le.appendChild(Pt)),Pt.style.transform=`translate(${Ce.left}px, ${Ce.top}px)`,Pt.style.width=`${Ce.width}px`,Pt.style.height=`${Ce.height}px`})}}};return Z.addEventListener("mousemove",d,{passive:!0}),()=>Z.removeEventListener("mousemove",d)},[F,H,Ao,qc]),(0,k.useEffect)(()=>{if(!F)return;let d=p=>{let w=Ao,M=Hn.current;if(Ao&&M){cl.current=!0;let $=Math.min(M.x,p.clientX),I=Math.min(M.y,p.clientY),T=Math.max(M.x,p.clientX),X=Math.max(M.y,p.clientY),_e=[];Z.querySelectorAll("button, a, input, img, p, h1, h2, h3, h4, h5, h6, li, label, td, th").forEach(ue=>{if(!(ue instanceof HTMLElement)||nn(ue,"[data-feedback-toolbar]")||nn(ue,"[data-annotation-marker]"))return;let we=Wt(ue);we.width>window.innerWidth*.8&&we.height>window.innerHeight*.5||we.width<10||we.height<10||we.left<T&&we.right>$&&we.top<X&&we.bottom>I&&_e.push({element:ue,rect:we})});let ge=_e.filter(({element:ue})=>!_e.some(({element:we})=>we!==ue&&ue.contains(we))),xe=p.clientX/window.innerWidth*100,$e=p.clientY+window.scrollY,Fe=(Cr(p)||kn.length>0)&&!H&&!ie;if(ge.length>0)if(Fe){let ue=[...kn];for(let{element:we,rect:Ee}of ge){if(ue.some(Le=>Le.element===we))continue;let{name:Xe,path:Ct,reactComponents:pn}=kc(we,Oo,oe);ue.push({element:we,rect:Ee,name:Xe,path:Ct,reactComponents:pn??void 0})}il.current=p.shiftKey,Cr(p)?Jo(ue):_l(ue)}else{let ue=ge.reduce((Le,{rect:Ce})=>({left:Math.min(Le.left,Ce.left),top:Math.min(Le.top,Ce.top),right:Math.max(Le.right,Ce.right),bottom:Math.max(Le.bottom,Ce.bottom)}),{left:1/0,top:1/0,right:-1/0,bottom:-1/0}),we=ge.slice(0,5).map(({element:Le})=>Gi(Le).name).join(", "),Ee=ge.length>5?` +${ge.length-5} more`:"",Xe=ge[0].element,Ct=cc(Xe),pn=dc(Xe);L({id:Date.now().toString(),x:xe,y:$e,clientY:p.clientY,element:`${ge.length} elements: ${we}${Ee}`,elementPath:"multi-select",boundingBox:{x:ue.left,y:ue.top+window.scrollY,width:ue.right-ue.left,height:ue.bottom-ue.top},isMultiSelect:!0,fullPath:ms(Xe),accessibility:uc(Xe),computedStyles:pn,computedStylesObj:Ct,nearbyElements:ac(Xe),cssClasses:ds(Xe),nearbyText:cs(Xe),sourceFile:Cc(Xe),attributes:ps(Xe,oe)})}else if(Fe&&!Cr(p))_l();else if(!Fe){let ue=Math.abs(T-$),we=Math.abs(X-I);ue>20&&we>20&&L({id:Date.now().toString(),x:xe,y:$e,clientY:p.clientY,element:"Area selection",elementPath:`region at (${Math.round($)}, ${Math.round(I)})`,boundingBox:{x:$,y:I+window.scrollY,width:ue,height:we},isMultiSelect:!0})}We(null)}else w&&(cl.current=!0);Or.current=null,Hn.current=null,Gc(!1),hi.current&&(hi.current.innerHTML="")};return Z.addEventListener("mouseup",d),()=>Z.removeEventListener("mouseup",d)},[F,Ao,H,ie,Oo,oe,kn,_l]);let vo=(0,k.useCallback)(async(d,p,w)=>{let M=et.webhookUrl||B;if(!M||!et.webhooksEnabled&&!w)return!1;try{return(await fetch(M,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({event:d,timestamp:Date.now(),url:typeof window<"u"?window.location.href:void 0,...p})})).ok}catch($){return console.warn("[Agentation] Webhook failed:",$),!1}},[B,et.webhookUrl,et.webhooksEnabled]),d1=(0,k.useCallback)(d=>{if(!H||H.isSubmitted)return;let p={id:H.id,x:H.x,y:H.y,comment:d,element:H.element,elementPath:H.elementPath,timestamp:Date.now(),selectedText:H.selectedText,boundingBox:H.boundingBox,nearbyText:H.nearbyText,cssClasses:H.cssClasses,isMultiSelect:H.isMultiSelect,isFixed:H.isFixed,fullPath:H.fullPath,accessibility:H.accessibility,computedStyles:H.computedStyles,nearbyElements:H.nearbyElements,reactComponents:H.reactComponents,sourceFile:H.sourceFile,attributes:H.attributes,frame:H.frame,elementBoundingBoxes:H.elementBoundingBoxes,...C&&jt?{sessionId:jt,url:typeof window<"u"?window.location.href:void 0,status:"pending"}:{}};Zn(w=>[...w,p]),L({...H,isSubmitted:!0}),ro.current=p.id,S?.(p),vo("annotation.add",{annotation:p}),sl(!0),window.getSelection()?.removeAllRanges(),C&&jt&&pe(async()=>{let w=await Ae(C,jt,p);if(o){let M=kr(e);N_(e,M.map($=>$.id===p.id?{...$,id:w.id}:$),jt)}!ae.current||Oe.current.has(p.id)||w.id!==p.id&&(Ss.current.set(w.id,p.id),ro.current===p.id&&(ro.current=w.id),Zn(M=>M.map($=>$.id===p.id?{...$,id:w.id}:$)),Br.current.delete(p.id)&&Br.current.add(w.id))}).catch(w=>{console.warn("[Agentation] Failed to sync annotation:",w)})},[H,S,vo,C,jt,pe,e,o]),nd=(0,k.useCallback)(()=>{sl(!0)},[]),od=(0,k.useCallback)(()=>{L(null),sl(!1)},[]),rd=(0,k.useCallback)(d=>{if(Oe.current.has(d))return;Oe.current.add(d);let p=Re.find(w=>w.id===d);ie?.id===d&&(Bo(!1),fi(!0)),Ms(w=>new Set(w).add(d)),p&&(b?.(p),vo("annotation.delete",{annotation:p})),C&&pe(()=>xc(C,kt.current.get(d)??d)).catch(w=>{console.warn("[Agentation] Failed to delete annotation from server:",w)})},[Re,ie,b,vo,C,pe]),Ts=(0,k.useCallback)(d=>{if(!d){Ie(null),mt(null),Pe([]);return}if(Ie(d.id),d.elementBoundingBoxes?.length){let p=[];for(let w of d.elementBoundingBoxes){let M=w.x+w.width/2,$=w.y+w.height/2-window.scrollY,I=_c(M,$,w);I&&p.push(I)}Pe(p),mt(null)}else if(d.boundingBox){let p=d.boundingBox,w=p.x+p.width/2,M=d.isFixed?p.y+p.height/2:p.y+p.height/2-window.scrollY,$=_c(w,M,p);if($){let I=Wt($),T=I.width/p.width,X=I.height/p.height;T<.5||X<.5?mt(null):mt($)}else mt(null);Pe([])}else mt(null),Pe([])},[]),u1=(0,k.useCallback)(d=>{if(!ie)return;let p={...ie,comment:d};In(p),Zn(w=>w.map(M=>M.id===ie.id?p:M)),N?.(p),vo("annotation.update",{annotation:p}),C&&pe(()=>$_(C,kt.current.get(ie.id)??ie.id,{comment:d})).catch(w=>{console.warn("[Agentation] Failed to update annotation on server:",w)}),Bo(rn.current||!!bn.current?.matches(":hover")),fi(!0)},[ie,N,vo,C,pe]),_1=(0,k.useCallback)(()=>{Bo(rn.current||!!bn.current?.matches(":hover")),fi(!0)},[]),f1=(0,k.useCallback)(()=>{go&&ie&&!H&&Ie(ie.id),In(null),An(null),nl([]),fi(!1)},[go,ie,H]),Ps=(0,k.useCallback)((d,p)=>{if(!d.length&&!p)return;ke(!0);let w={placements:[...di.current.placements,...d],rearrange:p??di.current.rearrange};di.current=w,mf(w.placements),gf(w.rearrange),clearTimeout(st.current),st.current=nt(()=>{zo(M=>M.filter($=>!w.placements.includes($))),yo(M=>M===w.rearrange?null:M),di.current={placements:[],rearrange:null},mf([]),gf(null),st.current=void 0,Je()},200)},[Je]),Ar=(0,k.useCallback)(()=>{if(!ae.current)return;let d=new Map(Ns.current.map(X=>[X.id,X])),p=[];for(let X of Re){let _e=d.get(kt.current.get(X.id)??X.id)??d.get(X.id);_e&&_e.comment===X.comment&&!Oe.current.has(_e.id)&&!p.includes(_e)&&p.push(_e)}let w=p.length,M=yf.current,$=ze.filter(X=>M.designPlacements.includes(X)&&!di.current.placements.includes(X)),I=me===M.rearrangeState&&me!==di.current.rearrange?me:null,T=oo.filter(X=>Uc.current.includes(X));if(!(w===0&&T.length===0&&$.length===0&&!I)){for(let X of p)Oe.current.add(X.id),Te.current.add(X.id),rt.current.add(X.id);if(Ms(X=>new Set([...X,...p.map(_e=>_e.id)])),E?.(p),vo("annotations.clear",{annotations:p}),C&&Promise.all(p.map(X=>pe(()=>xc(C,kt.current.get(X.id)??X.id)).catch(_e=>{console.warn("[Agentation] Failed to delete annotation from server:",_e)}))),ke(!0),n1(X=>X.filter(_e=>!T.includes(_e))),T.length>0&&T.length===Uc.current.length){let X=Yc.current;X?.getContext("2d")?.clearRect(0,0,X.width,X.height)}Ps($,I),yt===M.blankCanvas&&sn===M.wireframePurpose&&ze===M.designPlacements&&me===M.rearrangeState&&(yt&&uf(!1),sn&&ws(""),$r.current={rearrange:null,placements:[]},yc(e)),Je()}},[e,Re,oo,ze,me,yt,sn,E,vo,C,pe,Je,Ps]),id=(0,k.useCallback)(async()=>{let d=j.start(),p=typeof window<"u"?window.location.pathname+window.location.search+window.location.hash:e,w=Ve&&yt,M;if(w){if(ze.length===0&&!me&&!sn)return;M=r?Mc(p,r):""}else{if(M=og(Re,p,et.outputDetail,{appName:r}),!M&&oo.length===0&&ze.length===0&&!me)return;M||(M=Mc(p,r))}if(!w&&oo.length>0){let I=new Set;for(let ye of Re)ye.drawingIndex!=null&&I.add(ye.drawingIndex);let T=Yc.current;T&&(T.style.visibility="hidden");let X=[],_e=window.scrollY;for(let ye=0;ye<oo.length;ye++){if(I.has(ye))continue;let ge=oo[ye];if(ge.points.length<2)continue;let xe=ge.fixed?ge.points:ge.points.map(At=>({x:At.x,y:At.y-_e})),$e=1/0,Fe=1/0,ue=-1/0,we=-1/0;for(let At of xe)$e=Math.min($e,At.x),Fe=Math.min(Fe,At.y),ue=Math.max(ue,At.x),we=Math.max(we,At.y);let Ee=ue-$e,Xe=we-Fe,Ct=Math.hypot(Ee,Xe),pn=xe[0],Le=xe[xe.length-1],Ce=Math.hypot(Le.x-pn.x,Le.y-pn.y),tt,Pt=Ce<Ct*.35,bo=Ee/Math.max(Xe,1);if(Pt&&Ct>20){let At=Math.max(Ee,Xe)*.15,er=0;for(let Fr of xe){let p1=Fr.x-$e<At,m1=ue-Fr.x<At,g1=Fr.y-Fe<At,y1=we-Fr.y<At;(p1||m1)&&(g1||y1)&&er++}tt=er>xe.length*.15?"box":"circle"}else bo>3&&Xe<40?tt="underline":Ce>Ct*.5?tt="arrow":tt="drawing";let io=Math.min(10,xe.length),hl=Math.max(1,Math.floor(xe.length/io)),Os=new Set,Kt=[],pl=[pn];for(let At=hl;At<xe.length-1;At+=hl)pl.push(xe[At]);pl.push(Le);for(let At of pl){let er=Sc(At.x,At.y);if(!er||Os.has(er)||nn(er,"[data-feedback-toolbar]"))continue;Os.add(er);let{name:Fr}=Gi(er);Kt.includes(Fr)||Kt.push(Fr)}let As=`${Math.round($e)},${Math.round(Fe)} \u2192 ${Math.round(ue)},${Math.round(we)}`,mi;(tt==="circle"||tt==="box")&&Kt.length>0?mi=`${tt==="box"?"Boxed":"Circled"} **${Kt[0]}**${Kt.length>1?` (and ${Kt.slice(1).join(", ")})`:""} (region: ${As})`:tt==="underline"&&Kt.length>0?mi=`Underlined **${Kt[0]}** (${As})`:tt==="arrow"&&Kt.length>=2?mi=`Arrow from **${Kt[0]}** to **${Kt[Kt.length-1]}** (${Math.round(pn.x)},${Math.round(pn.y)} \u2192 ${Math.round(Le.x)},${Math.round(Le.y)})`:Kt.length>0?mi=`${tt==="arrow"?"Arrow":"Drawing"} near **${Kt.join("**, **")}** (region: ${As})`:mi=`Drawing at ${As}`,X.push(mi)}T&&(T.style.visibility=""),X.length>0&&(M+=`
**Drawings:**
`,X.forEach((ye,ge)=>{M+=`${ge+1}. ${ye}
`}))}if((ze.length>0||w&&sn)&&(M+=`
`+q0(ze,{width:window.innerWidth,height:window.innerHeight},{blankCanvas:yt,wireframePurpose:sn||void 0},et.outputDetail)),me){let I=K0(me,et.outputDetail,{width:window.innerWidth,height:window.innerHeight});I&&(M+=`
`+I)}if(M=rg(Re,M,s),!M){z(!1);return}let $=!h||await nw(M);g?.(M),j.isCurrent(d)&&(z($),$&&(j.schedule(d,()=>z(!1),2e3),et.autoClearAfterCopy&&j.schedule(d,Ar,500)))},[Re,oo,ze,me,yt,Ve,hf,sn,e,et.outputDetail,Oo,oe,et.autoClearAfterCopy,Ar,j,h,s,r,g]),ld=hg(et.webhookUrl)||hg(B||""),wo=v!=null||ld&&!et.webhooksEnabled,Ds=F?wo?337:297:44,sd=(0,k.useCallback)(async()=>{let d=ee.start(),p=typeof window<"u"?window.location.href:e,w=typeof window<"u"?window.location.pathname+window.location.search+window.location.hash:e,M=og(Re,w,et.outputDetail,{appName:r});if(!M&&ze.length===0&&!me)return;if(M||(M=Mc(w,r)),ze.length>0&&(M+=`
`+q0(ze,{width:window.innerWidth,height:window.innerHeight},{blankCanvas:yt,wireframePurpose:sn||void 0},et.outputDetail)),me){let X=K0(me,et.outputDetail,{width:window.innerWidth,height:window.innerHeight});X&&(M+=`
`+X)}Y("sending");let $=!0;try{await v?.(M,Re)}catch(X){console.warn("[Agentation] Submit callback failed:",X),$=!1}if(!ee.isCurrent(d))return;let I=ld?await vo("submit",{output:M,annotations:Re,url:p},!0):!0,T=$&&I&&wo;ee.isCurrent(d)&&(Y(T?"sent":"failed"),ee.schedule(d,()=>Y("idle"),2500),T&&et.autoClearAfterCopy&&ee.schedule(d,Ar,500))},[v,r,vo,Re,ze,me,yt,hf,e,et.outputDetail,Oo,oe,et.autoClearAfterCopy,Ar,ld,wo,ee]);(0,k.useEffect)(()=>{let p=(I=!1)=>{ll.current?.dragging&&(Cs.current=I,Nf(!1)),ll.current=null},w=I=>{let T=ll.current;if(!T)return;if((I.buttons&1)===0){p();return}let X=I.clientX-T.x,_e=I.clientY-T.y,ye=Math.sqrt(X*X+_e*_e);if(!T.dragging&&ye>10&&(T.dragging=!0,Nf(!0)),T.dragging){let ge=T.toolbarX+X,xe=T.toolbarY+_e,$e=20,Fe=337,ue=44,Ee=Fe-Ds,Xe=$e-Ee,Ct=window.innerWidth-$e-Fe;ge=Math.max(Xe,Math.min(Ct,ge)),xe=Math.max($e,Math.min(window.innerHeight-ue-$e,xe)),Xc({x:ge,y:xe})}},M=()=>p(!0),$=()=>p();return Z.addEventListener("mousemove",w),Z.addEventListener("mouseup",M,!0),window.addEventListener("blur",$),()=>{Z.removeEventListener("mousemove",w),Z.removeEventListener("mouseup",M,!0),window.removeEventListener("blur",$)}},[Ds]);let h1=(0,k.useCallback)(d=>{if(Cs.current=!1,ll.current=null,d.button!==0||d.target.closest("button")&&(d.target.closest("button")!==eo.current||F)||d.target.closest("[data-agentation-settings-panel]"))return;let p=d.currentTarget.parentElement;if(!p)return;let w=Wt(p);ll.current={x:d.clientX,y:d.clientY,toolbarX:w.left,toolbarY:w.top,dragging:!1}},[F]);(0,k.useLayoutEffect)(()=>{if(!Tt)return;let d=()=>{let $=Tt.x,I=Tt.y,_e=20-(337-Ds),ye=window.innerWidth-20-337;$=Math.max(_e,Math.min(ye,$)),I=Math.max(20,Math.min(window.innerHeight-44-20,I)),($!==Tt.x||I!==Tt.y)&&Xc({x:$,y:I})};return d(),window.addEventListener("resize",d),()=>window.removeEventListener("resize",d)},[Tt,Ds]),(0,k.useEffect)(()=>{if(!i)return;let d=w=>{if(w.defaultPrevented||w.isComposing||w.altKey)return;let M=w.composedPath()[0]||w.target,$=M.tagName==="INPUT"||M.tagName==="TEXTAREA"||M.tagName==="SELECT"||M.isContentEditable;if(w.key==="Escape"){if(c&&!H&&!ie&&(F||ln||Ve||Wn||kn.length)&&(w.preventDefault(),w.stopPropagation()),ln){w.preventDefault(),Nr(!1),Er.current?.focus();return}if(Ve){si?Rr(null):ul();return}if(Wn){Hc(!1);return}if(kn.length>0){Jo([]);return}H||ie||F&&(an(),Is())}if((w.metaKey||w.ctrlKey)&&w.shiftKey&&(w.key==="f"||w.key==="F")){w.preventDefault(),an(),F?Is():(eo.current?.blur(),On.current=!0,K(!0));return}!F||$||w.metaKey||w.ctrlKey||w.repeat||((w.key==="p"||w.key==="P")&&(w.preventDefault(),an(),td()),(w.key==="l"||w.key==="L")&&(w.preventDefault(),an(),Wn&&Hc(!1),ln&&Nr(!1),H&&nd(),Ve?ul():ed()),(w.key==="h"||w.key==="H")&&Re.length>0&&(w.preventDefault(),an(),qo(I=>!I)),(w.key==="c"||w.key==="C")&&(Re.length>0||ze.length>0||me)&&(w.preventDefault(),an(),id()),(w.key==="x"||w.key==="X")&&(Re.length>0||ze.length>0||me)&&(w.preventDefault(),an(),Ar(),ze.length>0&&zo([]),me&&yo(null)),(w.key==="s"||w.key==="S")&&Re.length>0&&wo&&te==="idle"&&(w.preventDefault(),an(),sd()))},p=!!c;return Z.addEventListener("keydown",d,p),()=>Z.removeEventListener("keydown",d,p)},[i,c,ie,F,Wn,Ve,si,ze,me,H,Re.length,wo,te,sd,td,id,Ar,kn,ln,Is,ed,ul]);let fl=Re.length>0,Bs=X5(),zs=Re.filter(d=>d.kind!=="placement"&&d.kind!=="rearrange"),Bf=zs.flatMap((d,p)=>{let w=Bs(d);return w?[{annotation:w,index:p}]:[]}),zf=H&&!H.isSubmitted?Bs({...H,comment:"",timestamp:0}):null,Of=[...he?Bf.map(d=>({...d,pending:!1})):[],...zf?[{annotation:zf,index:zs.length,pending:!0}]:[]];(0,k.useEffect)(()=>{let d=new Set(he&&!Ko?Bf.map(({annotation:p})=>p.id):[]);ro.current&&!d.has(ro.current)&&(ro.current=null);for(let p of Zo)d.has(p)||Jc(p)}),(0,k.useEffect)(()=>{ie&&Zo.has(ie.id)&&(Bo(!1),fi(!0))},[ie,Zo]);let Af=(0,k.useCallback)(d=>{!Me&&d.id!==ro.current&&Ts(d)},[Me,Ts]),Ff=(0,k.useCallback)(d=>{je===d&&Ts(null)},[je,Ts]),Wf=(0,k.useCallback)((d,p)=>{if(ie&&!zr){Es.current?.shake();return}jn&&od(),et.markerClickBehavior==="delete"?rd(d.id):$s(d,p)},[et.markerClickBehavior,rd,$s,jn,od,ie,zr]),Un=ie??(Ls&&!H&&!de?Re.find(d=>d.id===je&&!Zo.has(d.id)):null),jf=Fn?"Resume animations":"Pause animations",Hf=Ve?"Exit layout mode":"Layout mode",Uf=Ro?"Hide markers":"Show markers",Yf=s!=="markdown"&&!rg(Re,"",s),Qf=typeof s=="object"?`Copy ${s.attribute}`:s==="source"?"Copy source paths":s==="classes"?"Copy classes":Ve&&yt?"Copy layout":"Copy feedback",pi=F?0:-1;return!it||Ko?null:ht(f2,{host:"agentation-toolbar",className:q,children:[ht("style",{"data-agentation-styles":"toolbar",children:[bw,Cw]}),ht("div",{ref:Bt,className:O.positionContext,style:{display:"contents"},"data-agentation-theme":xo?"dark":"light","data-agentation-accent":et.annotationColorId,"data-agentation-root":"",children:[ce("div",{className:O.toolbar,"data-feedback-toolbar":!0,"data-agentation-toolbar":!0,"data-dragging":ks||void 0,style:Tt?{left:Tt.x,top:Tt.y,right:"auto",bottom:"auto"}:void 0,children:ht("div",{className:`${O.toolbarContainer} ${F?O.expanded:O.collapsed} ${Sf?O.entrance:""} ${To?O.hiding:""} ${wo?O.serverConnected:""}`,onMouseDown:h1,children:[ht("div",{className:`${O.controlsContent} ${F?O.visible:O.hidden} ${Tt&&Tt.y<100?O.tooltipBelow:""} ${df||ln?O.tooltipsHidden:""} ${Qc?O.tooltipsInSession:""}`,ref:d=>{mo.current=d,d?.toggleAttribute("inert",!F)},role:"group","aria-label":"Feedback controls","aria-hidden":!F,onMouseEnter:bf,onMouseLeave:kf,children:[ht("div",{className:`${O.buttonWrapper} ${Tt&&Tt.x<120?O.buttonWrapperAlignLeft:""}`,children:[ce("button",{className:O.controlButton,onClick:d=>{d.stopPropagation(),an(),td()},"data-active":Fn,"aria-label":jf,"aria-pressed":Fn,tabIndex:pi,children:ce(x2,{size:24,isPaused:Fn})}),ht("span",{className:O.buttonTooltip,children:[jf,i&&ce("span",{className:O.shortcut,children:"P"})]})]}),ht("div",{className:O.buttonWrapper,children:[ce("button",{className:`${O.controlButton} ${xo?"":O.light}`,onClick:d=>{d.stopPropagation(),an(),Wn&&Hc(!1),ln&&Nr(!1),H&&nd(),Ve?ul():ed()},"data-active":Ve,"aria-label":Hf,"aria-pressed":Ve,tabIndex:pi,style:Ve&&yt?{color:"#f97316",background:"rgba(249, 115, 22, 0.25)"}:void 0,children:ce(E2,{size:21})}),ht("span",{className:O.buttonTooltip,children:[Hf,i&&ce("span",{className:O.shortcut,children:"L"})]})]}),ht("div",{className:O.buttonWrapper,children:[ce("button",{className:O.controlButton,onClick:d=>{d.stopPropagation(),an(),qo(!Ro)},disabled:!fl||Ve,"aria-label":Uf,tabIndex:pi,children:ce(y2,{size:24,isOpen:Ro})}),ht("span",{className:O.buttonTooltip,children:[Uf,i&&ce("span",{className:O.shortcut,children:"H"})]})]}),ht("div",{className:O.buttonWrapper,children:[ce("button",{className:`${O.controlButton} ${R?O.statusShowing:""}`,onClick:d=>{d.stopPropagation(),an(),id()},disabled:Yf||(Ve&&yt?ze.length===0&&!me?.sections?.length:!fl&&oo.length===0&&ze.length===0&&!me?.sections?.length),"data-active":R,"aria-label":Qf,tabIndex:pi,children:ce(m2,{size:24,copied:R,tint:Ve&&yt&&(ze.length>0||me?.sections?.length)?"#f97316":void 0})}),ht("span",{className:O.buttonTooltip,children:[Yf?"No matching metadata":Qf,i&&ce("span",{className:O.shortcut,children:"C"})]})]}),ht("div",{className:`${O.buttonWrapper} ${O.sendButtonWrapper} ${F&&wo?O.sendButtonVisible:""}`,children:[ht("button",{className:`${O.controlButton} ${te==="sent"||te==="failed"?O.statusShowing:""}`,onClick:d=>{d.stopPropagation(),an(),sd()},disabled:!fl||!wo||te==="sending","data-no-hover":te==="sent"||te==="failed",tabIndex:F&&wo?0:-1,"aria-label":"Send Annotations","aria-hidden":!wo,children:[ce(g2,{size:24,state:te}),fl&&te==="idle"&&ce("span",{className:O.buttonBadge,children:Re.length})]}),ht("span",{className:O.buttonTooltip,children:["Send Annotations",i&&ce("span",{className:O.shortcut,children:"S"})]})]}),ht("div",{className:O.buttonWrapper,children:[ce("button",{className:O.controlButton,onClick:d=>{d.stopPropagation(),an(),Ar()},disabled:!fl&&oo.length===0&&ze.length===0&&!me?.sections?.length,"data-danger":!0,"aria-label":"Clear all",tabIndex:pi,children:ce(w2,{size:24})}),ht("span",{className:O.buttonTooltip,children:["Clear all",i&&ce("span",{className:O.shortcut,children:"X"})]})]}),ht("div",{className:O.buttonWrapper,children:[ce("button",{ref:Er,"aria-label":"Settings","aria-expanded":ln,tabIndex:pi,className:O.controlButton,onClick:d=>{d.stopPropagation(),an(),Ve&&ul(),G.current=!ln&&d.detail===0,Nr(!ln)},children:ce(v2,{size:24})}),C&&Pr!=="disconnected"&&ce("span",{className:`${O.mcpIndicator} ${O[Pr]} ${ln?O.hidden:""}`,title:Pr==="connected"?"MCP Connected":"MCP Connecting..."}),ce("span",{className:O.buttonTooltip,children:"Settings"})]}),ce("div",{className:O.divider}),ce("div",{className:O.togglePlaceholder,"aria-hidden":"true"})]}),ht("div",{className:`${O.buttonWrapper} ${O.toggleWrapper} ${Tt&&Tt.y<100?O.tooltipBelow:""} ${!F||df||ln?O.tooltipsHidden:""} ${Qc?O.tooltipsInSession:""} ${Tt&&typeof window<"u"&&Tt.x>window.innerWidth-120?O.buttonWrapperAlignRight:""}`,onMouseEnter:bf,onMouseLeave:kf,children:[ce("button",{ref:eo,type:"button",className:`${O.toggleContent} ${F?O.expandedToggle:""}`,"aria-label":F?"Exit":"Start feedback mode","aria-expanded":F,"aria-keyshortcuts":i?"Meta+Shift+F Control+Shift+F":void 0,title:F?void 0:i?"Start feedback mode (\u2318\u21E7F / Ctrl+Shift+F)":"Start feedback mode",onClick:d=>{if(Cs.current){Cs.current=!1,d.preventDefault();return}d.stopPropagation(),F?(an(),Is()):(d.currentTarget.blur(),On.current=d.detail===0,K(!0))},children:ht("span",{className:O.toggleIcon,children:[ce(P2,{active:F}),zs.length>0&&ce("span",{className:`${O.badge} ${F?O.fadeOut:""} ${Sf?O.entrance:""}`,children:zs.length})]})}),ht("span",{className:O.buttonTooltip,"aria-hidden":!F,children:["Exit",i&&ce("span",{className:O.shortcut,children:"Esc"})]})]}),ce(nv,{visible:Ve&&F,activeType:si,onSelect:d=>{Rr(si===d?null:d)},isDarkMode:xo,sectionCount:me?.sections.length??0,onDetectSections:()=>{let d=lv(),p=me?.sections??[],w=new Set(p.map(T=>T.selector)),M=d.filter(T=>!w.has(T.selector)),$=[...p,...M],I=[...me?.originalOrder??[],...M.map(T=>T.id)];yo({sections:$,originalOrder:I,detectedAt:Date.now()})},placementCount:ze.length,onClearPlacements:()=>{Ps(ze,me)},blankCanvas:yt,onBlankCanvasChange:d=>{let p={sections:[],originalOrder:[],detectedAt:Date.now()};d?(Ac.current={rearrange:me,placements:ze},yo($r.current.rearrange||p),zo($r.current.placements),Rr(null)):($r.current={rearrange:me,placements:ze},yo(Ac.current.rearrange||p),zo(Ac.current.placements)),uf(d)},wireframePurpose:sn,onWireframePurposeChange:ws,Tooltip:ii,onDragStart:(d,p)=>{p.preventDefault();let w=ne[d],M=null,$=!1,I=p.clientX,T=p.clientY,_e=p.target.closest("[data-feedback-toolbar]")?.getBoundingClientRect().top??window.innerHeight,ye=xe=>{let $e=xe.clientX-I,Fe=xe.clientY-T;if(!$&&(Math.abs($e)>4||Math.abs(Fe)>4)&&($=!0,M=document.createElement("div"),M.className=`${D.dragPreview}${yt?` ${D.dragPreviewWireframe}`:""}`,Bt.current?.appendChild(M)),!M)return;let ue=Math.max(0,_e-xe.clientY),we=Math.min(1,ue/180),Ee=1-Math.pow(1-we,2),Xe=28,Ct=20,pn=Math.min(140,w.width*.18),Le=Math.min(90,w.height*.18),Ce=Xe+(pn-Xe)*Ee,tt=Ct+(Le-Ct)*Ee;M.style.width=`${Ce}px`,M.style.height=`${tt}px`,M.style.left=`${xe.clientX-Ce/2}px`,M.style.top=`${xe.clientY-tt/2}px`,M.style.opacity=`${.5+.5*Ee}`,M.textContent=Ee>.25?d:""},ge=xe=>{if(window.removeEventListener("mousemove",ye),window.removeEventListener("mouseup",ge),M&&M.remove(),$){let $e=w.width,Fe=w.height,ue=window.scrollY,we=Math.max(0,xe.clientX-$e/2),Ee=Math.max(0,xe.clientY+ue-Fe/2),Xe={id:`dp-${Date.now()}-${Math.random().toString(36).slice(2,7)}`,type:d,x:we,y:Ee,width:$e,height:Fe,scrollY:ue,timestamp:Date.now()};zo(Ct=>[...Ct,Xe]),Rr(null),rl.current=new Set,pf(Ct=>Ct+1)}};window.addEventListener("mousemove",ye),window.addEventListener("mouseup",ge)}}),ce(yw,{settings:et,onSettingsChange:i1,isDarkMode:xo,onToggleTheme:l1,isDevMode:Ef,connectionStatus:Pr,endpoint:C,onExited:a1,isOpen:F&&ln,toolbarNearBottom:!!Tt&&Tt.y<230,settingsPage:Gg,onSettingsPageChange:cf,onHideToolbar:c1})]})}),(Ve||Ir)&&ce("div",{className:`${D.blankCanvas} ${_f?D.visible:""} ${Kg?D.gridActive:""}`,style:{"--canvas-opacity":ff},"data-feedback-toolbar":!0}),Ve&&yt&&_f&&ht("div",{className:D.wireframeNotice,"data-feedback-toolbar":!0,children:[ht("div",{className:D.wireframeOpacityRow,children:[ce("span",{className:D.wireframeOpacityLabel,children:"Toggle Opacity"}),ce("input",{type:"range",className:D.wireframeOpacitySlider,min:0,max:1,step:.01,value:ff,onChange:d=>qg(Number(d.target.value))})]}),ht("div",{className:D.wireframeNoticeTitleRow,children:[ce("span",{className:D.wireframeNoticeTitle,children:"Wireframe Mode"}),ce("span",{className:D.wireframeNoticeDivider}),ce("button",{className:D.wireframeStartOver,onClick:()=>{Ps(ze,me),$r.current={rearrange:null,placements:[]},ws(""),yc(e)},children:"Start Over"})]}),"Drag components onto the canvas.",ce("br",{}),"Copied output will only include the wireframed layout."]}),(Ve||Ir)&&ce(Kx,{placements:ze,onChange:zo,activeComponent:Ir?null:si,onActiveComponentChange:Rr,isDarkMode:xo,exiting:Ir,onInteractionChange:Jg,passthrough:!si,extraSnapRects:me?.sections.map(d=>d.currentRect),deselectSignal:Zg,clearingPlacements:ai,wireframe:yt,onSelectionChange:(d,p)=>{rl.current=d,p||(bs.current=new Set,t1(w=>w+1))},onDragMove:(d,p)=>{let w=bs.current;if(!(!w.size||!me)){if(!no.current){no.current=new Map;for(let M of me.sections)w.has(M.id)&&no.current.set(M.id,{x:M.currentRect.x,y:M.currentRect.y})}for(let M of me.sections){if(!w.has(M.id)||!no.current.get(M.id))continue;let I=Bt.current?.querySelector(`[data-rearrange-section="${M.id}"]`);I&&(I.style.transform=`translate(${d}px, ${p}px)`)}}},onDragEnd:(d,p,w)=>{let M=bs.current,$=no.current;if(no.current=null,!(!M.size||!me||!$)){for(let I of M){let T=Bt.current?.querySelector(`[data-rearrange-section="${I}"]`);T&&(T.style.transform="")}w&&yo(I=>I&&{...I,sections:I.sections.map(T=>{let X=$.get(T.id);return X?{...T,currentRect:{...T.currentRect,x:Math.max(0,X.x+d),y:Math.max(0,X.y+p)}}:T})})}}}),(Ve||Ir)&&me&&ce(cv,{rearrangeState:me,onChange:yo,isDarkMode:xo,exiting:Ir,blankCanvas:yt,extraSnapRects:ze.map(d=>({x:d.x,y:d.y,width:d.width,height:d.height})),clearing:me===ci,deselectSignal:e1,onSelectionChange:(d,p)=>{bs.current=d,p||(rl.current=new Set,pf(w=>w+1))},onDragMove:(d,p)=>{let w=rl.current;if(w.size){if(!no.current){no.current=new Map;for(let M of ze)w.has(M.id)&&no.current.set(M.id,{x:M.x,y:M.y})}for(let M of w){let $=Bt.current?.querySelector(`[data-design-placement="${M}"]`);$&&($.style.transform=`translate(${d}px, ${p}px)`)}}},onDragEnd:(d,p,w)=>{let M=rl.current,$=no.current;if(no.current=null,!(!M.size||!$)){for(let I of M){let T=Bt.current?.querySelector(`[data-design-placement="${I}"]`);T&&(T.style.transform="")}w&&zo(I=>I.map(T=>{let X=$.get(T.id);return X?{...T,x:Math.max(0,X.x+d),y:Math.max(0,X.y+p)}:T}))}}}),ce("canvas",{ref:Yc,className:`${O.drawCanvas} ${Wn?O.active:""}`,"aria-hidden":"true",style:{opacity:Ls?1:0,transition:"opacity 0.15s ease"},"data-feedback-toolbar":!0}),ce("div",{className:O.markersLayer,"data-feedback-toolbar":!0,children:Of.filter(({annotation:d})=>!d.isFixed).map(({annotation:d,index:p,pending:w},M,$)=>ce(ag,{annotation:d,pending:w,globalIndex:p,layerIndex:M,layerSize:$.length,isExiting:w?jn:Me,isClearing:Te.current.has(d.id),isAnimated:Br.current.has(d.id),isNew:ro.current===d.id,onEnterComplete:If,isHovered:!Me&&je===d.id,isRemoving:Zo.has(d.id),onRemoveComplete:Jc,isEditingAny:!!ie,renumberFrom:Ze,markerClickBehavior:et.markerClickBehavior,onHoverEnter:Af,onHoverLeave:Ff,onClick:Wf,onContextMenu:$s},Ss.current.get(d.id)??d.id))}),ce("div",{className:O.fixedMarkersLayer,"data-feedback-toolbar":!0,children:Of.filter(({annotation:d})=>d.isFixed).map(({annotation:d,index:p,pending:w},M,$)=>ce(ag,{annotation:d,pending:w,globalIndex:p,layerIndex:M,layerSize:$.length,isExiting:w?jn:Me,isClearing:Te.current.has(d.id),isAnimated:Br.current.has(d.id),isNew:ro.current===d.id,onEnterComplete:If,isHovered:!Me&&je===d.id,isRemoving:Zo.has(d.id),onRemoveComplete:Jc,isEditingAny:!!ie,renumberFrom:Ze,markerClickBehavior:et.markerClickBehavior,onHoverEnter:Af,onHoverLeave:Ff,onClick:Wf,onContextMenu:$s},Ss.current.get(d.id)??d.id))}),F&&Ke&&!H&&!ie&&!lf&&!Ao&&ce(vw,{x:Qe.x,y:Qe.y,elementName:Ke.elementName,reactComponents:Ke.reactComponents}),F&&ht("div",{className:O.overlay,"data-feedback-toolbar":!0,style:H||ie?{zIndex:"inherit"}:void 0,children:[Ke?.rect&&!H&&!lf&&!Ao&&ce("div",{className:`${O.hoverHighlight} ${O.enter}`,style:{left:Ke.rect.left,top:Ke.rect.top,width:Ke.rect.width,height:Ke.rect.height,borderColor:"color-mix(in srgb, var(--agentation-color-accent) 50%, transparent)",backgroundColor:"color-mix(in srgb, var(--agentation-color-accent) 4%, transparent)",...Ke.isPiercing?{borderStyle:"dashed"}:{}}}),kn.filter(d=>d.element.isConnected).map((d,p)=>{let w=Wt(d.element),M=kn.length>1;return ce("div",{className:M?O.multiSelectOutline:O.singleSelectOutline,style:{position:"fixed",left:w.left,top:w.top,width:w.width,height:w.height,...M?{}:{borderColor:"color-mix(in srgb, var(--agentation-color-accent) 60%, transparent)",backgroundColor:"color-mix(in srgb, var(--agentation-color-accent) 5%, transparent)"}}},p)}),je&&!H&&(()=>{let d=Re.find($=>$.id===je);if(!d?.boundingBox)return null;if(d.elementBoundingBoxes?.length)return at.length>0?at.filter($=>$.isConnected).map(($,I)=>{let T=Wt($);return ce("div",{className:`${O.multiSelectOutline} ${O.enter}`,style:{left:T.left,top:T.top,width:T.width,height:T.height}},`hover-outline-live-${I}`)}):d.elementBoundingBoxes.map(($,I)=>ce("div",{className:`${O.multiSelectOutline} ${O.enter}`,style:{left:$.x,top:$.y-Lr,width:$.width,height:$.height}},`hover-outline-${I}`));let p=gt&&gt.isConnected?Wt(gt):null,w=p?{x:p.left,y:p.top,width:p.width,height:p.height}:{x:d.boundingBox.x,y:d.isFixed?d.boundingBox.y:d.boundingBox.y-Lr,width:d.boundingBox.width,height:d.boundingBox.height},M=d.isMultiSelect;return ce("div",{className:`${M?O.multiSelectOutline:O.singleSelectOutline} ${O.enter}`,style:{left:w.x,top:w.y,width:w.width,height:w.height,...M?{}:{borderColor:"color-mix(in srgb, var(--agentation-color-accent) 60%, transparent)",backgroundColor:"color-mix(in srgb, var(--agentation-color-accent) 5%, transparent)"}}})})(),H&&ht(bc,{children:[H.multiSelectElements?.length?H.multiSelectElements.filter(d=>d.isConnected).map((d,p)=>{let w=Wt(d);return ce("div",{className:`${O.multiSelectOutline} ${jn?O.exit:O.enter}`,style:{left:w.left,top:w.top,width:w.width,height:w.height}},`pending-multi-${p}`)}):H.targetElement&&H.targetElement.isConnected?(()=>{let d=Wt(H.targetElement);return ce("div",{className:`${O.singleSelectOutline} ${jn?O.exit:O.enter}`,style:{left:d.left,top:d.top,width:d.width,height:d.height,borderColor:"color-mix(in srgb, var(--agentation-color-accent) 60%, transparent)",backgroundColor:"color-mix(in srgb, var(--agentation-color-accent) 5%, transparent)"}})})():H.boundingBox&&ce("div",{className:`${H.isMultiSelect?O.multiSelectOutline:O.singleSelectOutline} ${jn?O.exit:O.enter}`,style:{left:H.boundingBox.x,top:H.boundingBox.y-Lr,width:H.boundingBox.width,height:H.boundingBox.height,...H.isMultiSelect?{}:{borderColor:"color-mix(in srgb, var(--agentation-color-accent) 60%, transparent)",backgroundColor:"color-mix(in srgb, var(--agentation-color-accent) 5%, transparent)"}}}),(()=>{let d=Bs(H)??H,p=d.x,w=d.isFixed?d.y:d.y-Lr;return ce(bc,{children:ce(J_,{ref:Kc,element:H.element,selectedText:H.selectedText,allowEmpty:typeof s=="object"&&!!H.attributes?.[s.attribute],onOpenSource:a&&H.sourceFile?()=>a(H.sourceFile):void 0,computedStyles:H.computedStylesObj,placeholder:typeof s=="object"&&H.attributes?.[s.attribute]?"Add a note (optional)":H.element==="Area selection"?"What should change in this area?":H.isMultiSelect?"Feedback for this group of elements...":"What should change?",onSubmit:d1,onExitComplete:od,onCancel:nd,isExiting:jn,lightMode:!xo,accentColor:H.isMultiSelect?"var(--agentation-color-green)":"var(--agentation-color-accent)",style:{left:Math.max(160,Math.min(window.innerWidth-160,p/100*window.innerWidth)),...w>window.innerHeight-290?{bottom:window.innerHeight-w+20}:{top:w+20}}},H.id)})})()]}),ie&&ce(bc,{children:ie.elementBoundingBoxes?.length?tl.length>0?tl.filter(d=>d.isConnected).map((d,p)=>{let w=Wt(d);return ce("div",{className:`${O.multiSelectOutline} ${O.enter}`,style:{left:w.left,top:w.top,width:w.width,height:w.height}},`edit-multi-live-${p}`)}):ie.elementBoundingBoxes.map((d,p)=>ce("div",{className:`${O.multiSelectOutline} ${O.enter}`,style:{left:d.x,top:d.y-Lr,width:d.width,height:d.height}},`edit-multi-${p}`)):(()=>{let d=to&&to.isConnected?Wt(to):null,p=d?{x:d.left,y:d.top,width:d.width,height:d.height}:ie.boundingBox?{x:ie.boundingBox.x,y:ie.isFixed?ie.boundingBox.y:ie.boundingBox.y-Lr,width:ie.boundingBox.width,height:ie.boundingBox.height}:null;return p?ce("div",{className:`${ie.isMultiSelect?O.multiSelectOutline:O.singleSelectOutline} ${O.enter}`,style:{left:p.x,top:p.y,width:p.width,height:p.height,...ie.isMultiSelect?{}:{borderColor:"color-mix(in srgb, var(--agentation-color-accent) 60%, transparent)",backgroundColor:"color-mix(in srgb, var(--agentation-color-accent) 5%, transparent)"}}}):null})()}),Ao&&ht(bc,{children:[ce("div",{ref:al,className:O.dragSelection}),ce("div",{ref:hi,className:O.highlightsContainer})]})]}),ce(sw,{ref:Es,annotation:Un?Bs(Un)??ie:null,editing:!!ie,exiting:zr,restorePreview:go,scrollY:Lr,lightMode:!xo,onExited:f1,editorProps:Un?{element:Un.element,selectedText:Un.selectedText,allowEmpty:typeof s=="object"&&!!Un.attributes?.[s.attribute],onOpenSource:a&&Un.sourceFile?()=>a(Un.sourceFile):void 0,computedStyles:i2(Un.computedStyles),placeholder:"Edit your feedback...",initialValue:Un.comment,submitLabel:"Save",onSubmit:u1,onCancel:_1,onDelete:()=>rd(Un.id),accentColor:Un.isMultiSelect?"var(--agentation-color-green)":"var(--agentation-color-accent)"}:void 0})]})]})}var el=document.getElementById("agentation-root");el||(el=document.createElement("div"),el.id="agentation-root");el.parentElement===null&&document.body.appendChild(el);(0,Vg.createRoot)(el).render((0,Qg.createElement)(Yg,{appName:"Spaxel"}));
/*! Bundled license information:

react/cjs/react.production.min.js:
  (**
   * @license React
   * react.production.min.js
   *
   * Copyright (c) Facebook, Inc. and its affiliates.
   *
   * This source code is licensed under the MIT license found in the
   * LICENSE file in the root directory of this source tree.
   *)

scheduler/cjs/scheduler.production.min.js:
  (**
   * @license React
   * scheduler.production.min.js
   *
   * Copyright (c) Facebook, Inc. and its affiliates.
   *
   * This source code is licensed under the MIT license found in the
   * LICENSE file in the root directory of this source tree.
   *)

react-dom/cjs/react-dom.production.min.js:
  (**
   * @license React
   * react-dom.production.min.js
   *
   * Copyright (c) Facebook, Inc. and its affiliates.
   *
   * This source code is licensed under the MIT license found in the
   * LICENSE file in the root directory of this source tree.
   *)
*/
